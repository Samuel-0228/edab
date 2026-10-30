export const fallbackTriggers = [
  { value: 'telegram', label: 'Telegram (Bot Messages & Channel Posts)' },
  { value: 'github', label: 'GitHub (Issues, Pull Requests, Push)' },
  { value: 'webhook', label: 'Inbound Webhook' },
  { value: 'rss', label: 'RSS / Atom Feed Poller' },
  { value: 'discord', label: 'Discord Interactions' },
  { value: 'schedule', label: 'Schedule (Periodic Cron / Interval)' },
  { value: 'all', label: 'Any Ingested Event' }
];

export const fallbackActions = [
  { value: 'discord', label: 'Discord (Channel Webhook / Embed)' },
  { value: 'telegram', label: 'Telegram (Send / Copy Message)' },
  { value: 'http', label: 'Generic REST HTTP API (SSRF Safe)' },
  { value: 'github', label: 'GitHub API (Create Issue / Comment)' }
];

export const conditionOperators = [
  { value: '', label: '(No Condition — Passthrough Always True)' },
  { value: 'contains', label: 'contains' },
  { value: 'equals', label: 'equals' },
  { value: 'not_equals', label: 'not equals' },
  { value: 'not_contains', label: 'not contains' },
  { value: 'starts_with', label: 'starts with' },
  { value: 'ends_with', label: 'ends with' },
  { value: 'matches', label: 'regex matches' },
  { value: 'exists', label: 'field exists' },
  { value: 'greater_than', label: '> greater than' },
  { value: 'less_than', label: '< less than' }
];

export const transformOptions = [
  { value: 'passthrough', label: 'Passthrough (Preserve Raw Event)' },
  { value: 'template', label: 'Template String' },
  { value: 'mapping', label: 'Field Map Dictionary' }
];
