package cli

import (
	"fmt"
	"io"
)

// GenerateCompletion writes an auto-completion script for the specified shell.
func GenerateCompletion(w io.Writer, shell string) error {
	switch shell {
	case "bash":
		_, err := fmt.Fprint(w, bashCompletionScript)
		return err
	case "zsh":
		_, err := fmt.Fprint(w, zshCompletionScript)
		return err
	case "fish":
		_, err := fmt.Fprint(w, fishCompletionScript)
		return err
	default:
		return fmt.Errorf("unsupported shell %q (supported: bash, zsh, fish)", shell)
	}
}

const bashCompletionScript = `#!/usr/bin/env bash
# Kerob CLI Bash Completion

_kerob_completions() {
    local cur prev words cword
    _init_completion || return

    local commands="version login tenant workflow event execution logs dlq completion help"
    local workflow_sub="list get validate import export deploy publish rollback diff"
    local tenant_sub="list select current"
    local event_sub="list get replay"
    local execution_sub="list get retry pause resume cancel compensate"
    local dlq_sub="list retry discard"
    local global_flags="--json --endpoint --token --tenant -h --help"

    if [[ ${cword} -eq 1 ]]; then
        COMPREPLY=( $(compgen -W "${commands} ${global_flags}" -- "${cur}") )
        return 0
    fi

    case "${words[1]}" in
        workflow)
            if [[ ${cword} -eq 2 ]]; then
                COMPREPLY=( $(compgen -W "${workflow_sub}" -- "${cur}") )
            fi
            ;;
        tenant)
            if [[ ${cword} -eq 2 ]]; then
                COMPREPLY=( $(compgen -W "${tenant_sub}" -- "${cur}") )
            fi
            ;;
        event)
            if [[ ${cword} -eq 2 ]]; then
                COMPREPLY=( $(compgen -W "${event_sub}" -- "${cur}") )
            fi
            ;;
        execution)
            if [[ ${cword} -eq 2 ]]; then
                COMPREPLY=( $(compgen -W "${execution_sub}" -- "${cur}") )
            fi
            ;;
        dlq)
            if [[ ${cword} -eq 2 ]]; then
                COMPREPLY=( $(compgen -W "${dlq_sub}" -- "${cur}") )
            fi
            ;;
        completion)
            if [[ ${cword} -eq 2 ]]; then
                COMPREPLY=( $(compgen -W "bash zsh fish" -- "${cur}") )
            fi
            ;;
    esac
}

complete -F _kerob_completions kerob
`

const zshCompletionScript = `#compdef kerob

_kerob() {
    local -a commands
    commands=(
        'version:Display Kerob CLI version and build details'
        'login:Configure authentication credentials and endpoint'
        'tenant:Manage and select active organizational tenant'
        'workflow:Inspect, validate, import, export, and deploy workflows'
        'event:Inspect and replay automation pipeline events'
        'execution:Inspect, retry, cancel, pause, resume, and compensate executions'
        'logs:Stream and inspect execution step audit logs'
        'dlq:Inspect, retry, and discard dead-letter queue records'
        'completion:Generate shell auto-completion script'
    )

    _arguments -C \
        '--json[Enable script-friendly JSON output]' \
        '--endpoint[Kerob server endpoint URL]:endpoint:_urls' \
        '--token[API authentication Bearer token]:token:' \
        '--tenant[Tenant context identifier]:tenant:' \
        '1: :->command' \
        '*:: :->args'

    case $state in
        command)
            _describe -t commands 'kerob command' commands
            ;;
        args)
            case $words[1] in
                workflow)
                    local -a wf_sub
                    wf_sub=(
                        'list:List workflows'
                        'get:Get workflow details'
                        'validate:Validate workflow schema and secret safety'
                        'import:Import workflow from file'
                        'export:Export workflow to JSON/YAML'
                        'deploy:GitOps workflow deployment'
                        'publish:Publish/enable a workflow'
                        'rollback:Rollback workflow to version snapshot'
                        'diff:Diff two workflow versions'
                    )
                    _describe -t wf_sub 'workflow subcommand' wf_sub
                    ;;
                tenant)
                    local -a tenant_sub
                    tenant_sub=('list:List tenants' 'select:Select active tenant' 'current:Show current tenant')
                    _describe -t tenant_sub 'tenant subcommand' tenant_sub
                    ;;
                event)
                    local -a ev_sub
                    ev_sub=('list:List events' 'get:Get event payload' 'replay:Replay event')
                    _describe -t ev_sub 'event subcommand' ev_sub
                    ;;
                execution)
                    local -a exec_sub
                    exec_sub=('list:List executions' 'get:Get execution run' 'retry:Retry execution' 'pause:Pause execution' 'resume:Resume execution' 'cancel:Cancel execution' 'compensate:Compensate execution')
                    _describe -t exec_sub 'execution subcommand' exec_sub
                    ;;
                dlq)
                    local -a dlq_sub
                    dlq_sub=('list:List dead-letter failures' 'retry:Retry failure' 'discard:Discard failure')
                    _describe -t dlq_sub 'dlq subcommand' dlq_sub
                    ;;
            esac
            ;;
    esac
}

_kerob "$@"
`

const fishCompletionScript = `# Kerob CLI Fish Completion

complete -c kerob -f

complete -c kerob -n "__fish_use_subcommand" -a version -d "Display Kerob CLI version"
complete -c kerob -n "__fish_use_subcommand" -a login -d "Configure authentication credentials"
complete -c kerob -n "__fish_use_subcommand" -a tenant -d "Manage organizational tenants"
complete -c kerob -n "__fish_use_subcommand" -a workflow -d "Workflow management and GitOps"
complete -c kerob -n "__fish_use_subcommand" -a event -d "Inspect and replay events"
complete -c kerob -n "__fish_use_subcommand" -a execution -d "Manage workflow execution runs"
complete -c kerob -n "__fish_use_subcommand" -a logs -d "View step audit logs"
complete -c kerob -n "__fish_use_subcommand" -a dlq -d "Manage dead-letter records"
complete -c kerob -n "__fish_use_subcommand" -a completion -d "Generate completion script"

complete -c kerob -l json -d "Enable script-friendly JSON output"
complete -c kerob -l endpoint -d "Server endpoint URL"
complete -c kerob -l token -d "Bearer API token"
complete -c kerob -l tenant -d "Tenant identifier"
`
