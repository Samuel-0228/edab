import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import { mount, unmount } from 'svelte';
import SchemaConfigForm from './SchemaConfigForm.svelte';
import type { IntegrationJSONSchema } from '../../api/types';

describe('SchemaConfigForm Component', () => {
  let target: HTMLElement;
  let instance: Record<string, unknown> | null = null;

  beforeEach(() => {
    target = document.createElement('div');
    document.body.appendChild(target);
  });

  afterEach(() => {
    if (instance) {
      unmount(instance);
      instance = null;
    }
    if (target && target.parentNode) {
      target.parentNode.removeChild(target);
    }
  });

  const sampleSchema: IntegrationJSONSchema = {
    type: 'object',
    required: ['chat_id', 'text'],
    properties: {
      chat_id: {
        type: 'string',
        title: 'Chat / Channel ID',
        description: 'Telegram chat or channel username / numeric ID'
      },
      text: {
        type: 'string',
        title: 'Message Text',
        description: 'Content of the message'
      },
      parse_mode: {
        type: 'string',
        title: 'Parse Mode',
        enum: ['MarkdownV2', 'HTML', 'Markdown'],
        default: 'MarkdownV2'
      },
      retries: {
        type: 'integer',
        title: 'Retry Count',
        default: 3
      }
    }
  };

  it('renders form view fields according to schema properties', async () => {
    instance = mount(SchemaConfigForm, {
      target,
      props: {
        schema: sampleSchema,
        value: JSON.stringify({ chat_id: '-100123', text: 'Hello World' })
      }
    });

    await new Promise((r) => setTimeout(r, 20));

    expect(target.textContent).toContain('Chat / Channel ID');
    expect(target.textContent).toContain('Message Text');
    expect(target.textContent).toContain('Parse Mode');
    expect(target.textContent).toContain('Retry Count');
    expect(target.textContent).toContain('Schema Driven');
  });

  it('allows toggling between Form View and Raw JSON view', async () => {
    instance = mount(SchemaConfigForm, {
      target,
      props: {
        schema: sampleSchema,
        value: JSON.stringify({ chat_id: 'test_chat' })
      }
    });

    await new Promise((r) => setTimeout(r, 20));

    const rawBtn = Array.from(target.querySelectorAll('button')).find((b) =>
      b.textContent?.includes('Raw JSON')
    );
    expect(rawBtn).toBeTruthy();
    rawBtn?.click();

    await new Promise((r) => setTimeout(r, 20));

    const formBtn = Array.from(target.querySelectorAll('button')).find((b) =>
      b.textContent?.includes('Form View')
    );
    expect(formBtn).toBeTruthy();
    formBtn?.click();

    await new Promise((r) => setTimeout(r, 20));
    expect(target.textContent).toContain('Chat / Channel ID');
  });

  it('gracefully handles missing schema by defaulting to json view', async () => {
    instance = mount(SchemaConfigForm, {
      target,
      props: {
        schema: null,
        value: '{"foo":"bar"}'
      }
    });

    await new Promise((r) => setTimeout(r, 20));
    expect(target.textContent).not.toContain('Schema Driven');
  });
});
