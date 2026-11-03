import type { Workflow, WorkflowCreateInput, DiffResult, DeployOptions, DeployResult, WorkflowDeployResult } from './types.js';
import type { KerobClient } from './client.js';

export interface SecretScanViolation {
  stepId?: string;
  field: string;
  reason: string;
}

// Patterns matching accidental plaintext credentials
const SENSITIVE_PATTERNS = [
  { pattern: /\bAKIA[0-9A-Z]{16}\b/, name: 'AWS Access Key' },
  { pattern: /\bghp_[0-9a-zA-Z]{36}\b/, name: 'GitHub Personal Access Token' },
  { pattern: /\bgho_[0-9a-zA-Z]{36}\b/, name: 'GitHub OAuth Token' },
  { pattern: /-----BEGIN [A-Z ]*PRIVATE KEY-----/, name: 'Private Key PEM' },
  { pattern: /\beyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\b/, name: 'JWT Bearer Token' }
];

const SUSPICIOUS_KEYS = ['password', 'secret', 'api_key', 'apikey', 'private_key', 'token', 'auth_token'];

/**
 * Validates that workflow files do not contain hardcoded credentials or secrets.
 * Secrets must be referenced by name (e.g. credential_id or {{ secret.NAME }}).
 */
export function validateWorkflowSecrets(wf: WorkflowCreateInput | Workflow): SecretScanViolation[] {
  const violations: SecretScanViolation[] = [];

  function checkValue(val: unknown, path: string, stepId?: string) {
    if (typeof val === 'string') {
      // Check for template reference (which is allowed and expected)
      if (val.includes('{{') && val.includes('}}')) {
        return;
      }
      for (const { pattern, name } of SENSITIVE_PATTERNS) {
        if (pattern.test(val)) {
          violations.push({
            stepId,
            field: path,
            reason: `Detected plaintext ${name}. Secrets must be referenced by credential_id or {{ secret.NAME }}`
          });
          return;
        }
      }
      const lastKey = path.split('.').pop()?.toLowerCase() || '';
      if (SUSPICIOUS_KEYS.includes(lastKey) && val.length > 8 && !val.startsWith('ref:') && !val.startsWith('env:')) {
        violations.push({
          stepId,
          field: path,
          reason: `Field '${lastKey}' appears to contain hardcoded secret value. Use credential_id or {{ secret.NAME }} instead.`
        });
      }
    } else if (typeof val === 'object' && val !== null) {
      for (const [k, v] of Object.entries(val)) {
        const subPath = path ? `${path}.${k}` : k;
        checkValue(v, subPath, stepId);
      }
    }
  }

  if (wf.trigger && wf.trigger.config) {
    checkValue(wf.trigger.config, 'trigger.config');
  }

  if (Array.isArray(wf.actions)) {
    for (const act of wf.actions) {
      if (act.config) {
        checkValue(act.config, `action.${act.id || act.name}.config`, act.id || act.name);
      }
    }
  }

  return violations;
}

/**
 * Computes structural differences between a local workflow definition and remote workflow.
 */
export function diffWorkflow(local: WorkflowCreateInput | Workflow, remote?: Workflow | null): DiffResult {
  const name = local.name;
  const workflowId = ('id' in local && local.id) ? local.id : (remote?.id || 'new');

  if (!remote) {
    return {
      workflow_id: workflowId,
      name,
      has_changes: true,
      changes: ['Workflow is new (not present on remote server)'],
      diff_text: `+ New workflow: ${name}`
    };
  }

  const changes: string[] = [];

  if (local.name !== remote.name) {
    changes.push(`Name changed: "${remote.name}" -> "${local.name}"`);
  }
  if ((local.description || '') !== (remote.description || '')) {
    changes.push(`Description changed: "${remote.description || ''}" -> "${local.description || ''}"`);
  }
  if (local.enabled !== undefined && local.enabled !== remote.enabled) {
    changes.push(`Enabled status changed: ${remote.enabled} -> ${local.enabled}`);
  }
  if (local.trigger.type !== remote.trigger.type) {
    changes.push(`Trigger type changed: ${remote.trigger.type} -> ${local.trigger.type}`);
  }
  if (local.actions.length !== remote.actions.length) {
    changes.push(`Action count changed: ${remote.actions.length} -> ${local.actions.length} steps`);
  } else {
    for (let i = 0; i < local.actions.length; i++) {
      const la = local.actions[i];
      const ra = remote.actions[i];
      if (la.id !== ra.id || la.integration !== ra.integration || la.action_type !== ra.action_type) {
        changes.push(`Step ${i + 1} (${la.name || la.id}) modified`);
      }
    }
  }

  return {
    workflow_id: workflowId,
    name,
    has_changes: changes.length > 0,
    changes,
    diff_text: changes.join('\n')
  };
}

/**
 * Deploys a set of workflows following GitOps principles (validation, secret check, diff, dry-run).
 */
export async function deployWorkflows(
  client: KerobClient,
  workflows: Array<WorkflowCreateInput & { id?: string }>,
  options: DeployOptions = {}
): Promise<DeployResult> {
  const results: WorkflowDeployResult[] = [];

  for (const wf of workflows) {
    // 1. Secret check
    const secretViolations = validateWorkflowSecrets(wf);
    if (secretViolations.length > 0) {
      results.push({
        workflow_id: wf.id || 'unknown',
        name: wf.name,
        action: 'skipped',
        error: `Secret policy violation: ${secretViolations.map((v) => `${v.field}: ${v.reason}`).join('; ')}`
      });
      continue;
    }

    // 2. Fetch remote state
    let remote: Workflow | null = null;
    if (wf.id) {
      try {
        remote = await client.getWorkflow(wf.id);
      } catch {
        remote = null;
      }
    } else {
      // Find by name
      try {
        const list = await client.listWorkflows();
        remote = list.find((item) => item.name === wf.name) || null;
      } catch {
        remote = null;
      }
    }

    const diff = diffWorkflow(wf, remote);

    if (!diff.has_changes) {
      results.push({
        workflow_id: remote?.id || wf.id || 'unknown',
        name: wf.name,
        action: 'unchanged',
        diff_summary: 'No changes detected'
      });
      continue;
    }

    if (options.dryRun) {
      results.push({
        workflow_id: remote?.id || wf.id || 'new',
        name: wf.name,
        action: remote ? 'updated' : 'created',
        diff_summary: diff.diff_text
      });
      continue;
    }

    if (options.requireApproval && !options.approved) {
      results.push({
        workflow_id: remote?.id || wf.id || 'new',
        name: wf.name,
        action: 'skipped',
        error: 'Deployment requires explicit approval (--approve)'
      });
      continue;
    }

    // Apply deployment
    try {
      if (remote) {
        const updated = await client.updateWorkflow(remote.id, wf);
        results.push({
          workflow_id: updated.id,
          name: updated.name,
          action: 'updated',
          diff_summary: diff.diff_text
        });
      } else {
        const created = await client.createWorkflow(wf);
        results.push({
          workflow_id: created.id,
          name: created.name,
          action: 'created',
          diff_summary: 'Created new workflow'
        });
      }
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err);
      results.push({
        workflow_id: remote?.id || wf.id || 'unknown',
        name: wf.name,
        action: 'skipped',
        error: msg
      });
    }
  }

  const success = results.every((r) => !r.error);
  return {
    success,
    dry_run: !!options.dryRun,
    workflows: results,
    summary: `Processed ${results.length} workflow(s): ${results.filter((r) => r.action === 'created').length} created, ${results.filter((r) => r.action === 'updated').length} updated, ${results.filter((r) => r.action === 'unchanged').length} unchanged, ${results.filter((r) => !!r.error).length} failed.`
  };
}
