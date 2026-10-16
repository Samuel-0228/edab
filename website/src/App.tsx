import { useState } from 'react';
import {
  ArrowRight,
  Check,
  Copy,
  Terminal,
  ExternalLink,
  GitBranch,
  Layers,
  Database,
  Send,
  Radio,
  ShieldCheck,
  Cpu,
  Menu,
  X,
  ChevronRight
} from 'lucide-react';

const REPO_URL = "https://github.com/Samuel-0228/EDA-backend-boilerplate";

export default function App() {
  const [copiedCmd, setCopiedCmd] = useState<string | null>(null);
  const [activeAdapterTab, setActiveAdapterTab] = useState<'source' | 'destination' | 'event' | 'registry'>('source');
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false);

  const copyToClipboard = (text: string, id: string) => {
    navigator.clipboard.writeText(text);
    setCopiedCmd(id);
    setTimeout(() => setCopiedCmd(null), 2000);
  };

  const quickstartCommands = `git clone ${REPO_URL}.git
cd EDA-backend-boilerplate
cp .env.example .env
docker compose up -d`;

  const sourceAdapterCode = `// internal/adapters/sources/source.go
type SourceAdapter interface {
    Name() string
    Connect(ctx context.Context, config json.RawMessage) error
    Health(ctx context.Context) error
    Poll(ctx context.Context) ([]RawEvent, error)
    Normalize(ctx context.Context, sourceID string, raw []byte) (*event.Event, error)
}`;

  const destinationAdapterCode = `// internal/adapters/destinations/destination.go
type DestinationAdapter interface {
    Name() string
    Connect(ctx context.Context, config json.RawMessage) error
    Health(ctx context.Context) error
    Deliver(ctx context.Context, ev *event.Event) error
}`;

  const eventModelCode = `// internal/core/event/event.go
type Event struct {
    ID              string          \`json:"id"\`
    SourceID        string          \`json:"source_id"\`
    ExternalEventID string          \`json:"external_event_id"\`
    EventType       string          \`json:"event_type"\`
    Payload         json.RawMessage \`json:"payload"\`
    Metadata        json.RawMessage \`json:"metadata"\`
    CreatedAt       time.Time       \`json:"created_at"\`
}`;

  const registryCode = `// cmd/server/main.go
srcRegistry := sources.NewRegistry()
srcRegistry.Register("webhook", func() sources.SourceAdapter { 
    return sources.NewWebhookSourceAdapter() 
})

destRegistry := destinations.NewRegistry()
destRegistry.Register("webhook", func() destinations.DestinationAdapter { 
    return destinations.NewWebhookDestinationAdapter() 
})
destRegistry.Register("stdout", func() destinations.DestinationAdapter { 
    return destinations.NewStdoutDestinationAdapter() 
})`;

  return (
    <div className="min-h-screen bg-black text-zinc-100 flex flex-col font-sans selection:bg-zinc-800 selection:text-white">
      {/* 1. Minimal Navigation */}
      <header className="sticky top-0 z-50 bg-black/90 backdrop-blur-sm border-b border-zinc-800/80">
        <div className="max-w-6xl mx-auto px-6 h-16 flex items-center justify-between">
          <div className="flex items-center space-x-3">
            <a href="#" className="flex items-center space-x-2.5 text-white font-mono tracking-tight text-sm font-semibold hover:text-zinc-300 transition-colors">
              <span className="w-6 h-6 border border-zinc-700 bg-zinc-900 rounded flex items-center justify-center text-xs">
                E
              </span>
              <span>EDA ENGINE</span>
            </a>
            <span className="hidden sm:inline-block text-[11px] font-mono text-zinc-500 uppercase tracking-wider px-2 py-0.5 border border-zinc-800 rounded">
              v0.1.0
            </span>
          </div>

          <nav className="hidden md:flex items-center space-x-7 text-xs font-mono text-zinc-400">
            <a href="#what-it-is" className="hover:text-white transition-colors">Concept</a>
            <a href="#how-it-works" className="hover:text-white transition-colors">Pipeline</a>
            <a href="#architecture" className="hover:text-white transition-colors">Architecture</a>
            <a href="#use-cases" className="hover:text-white transition-colors">Use Cases</a>
            <a href="#adapters" className="hover:text-white transition-colors">Adapters</a>
            <a href="#quickstart" className="hover:text-white transition-colors">Docs</a>
          </nav>

          <div className="hidden md:flex items-center space-x-3">
            <a
              href={REPO_URL}
              target="_blank"
              rel="noreferrer"
              className="flex items-center space-x-2 px-3 py-1.5 text-xs font-mono text-zinc-300 hover:text-white border border-zinc-800 hover:border-zinc-700 rounded transition-colors"
            >
              <svg className="w-4 h-4 fill-current" viewBox="0 0 24 24">
                <path fillRule="evenodd" clipRule="evenodd" d="M12 2C6.477 2 2 6.484 2 12.017c0 4.425 2.865 8.18 6.839 9.504.5.092.682-.217.682-.483 0-.237-.008-.868-.013-1.703-2.782.605-3.369-1.343-3.369-1.343-.454-1.158-1.11-1.466-1.11-1.466-.908-.62.069-.608.069-.608 1.003.07 1.53 1.032 1.53 1.032.892 1.53 2.341 1.088 2.91.832.092-.647.35-1.088.636-1.338-2.22-.253-4.555-1.113-4.555-4.951 0-1.093.39-1.988 1.029-2.688-.103-.253-.446-1.272.098-2.65 0 0 .84-.27 2.75 1.026A9.564 9.564 0 0112 6.844c.85.004 1.705.115 2.504.337 1.909-1.296 2.747-1.027 2.747-1.027.546 1.379.202 2.398.1 2.651.64.7 1.028 1.595 1.028 2.688 0 3.848-2.339 4.695-4.566 4.943.359.309.678.92.678 1.855 0 1.338-.012 2.419-.012 2.747 0 .268.18.58.688.482A10.019 10.019 0 0022 12.017C22 6.484 17.522 2 12 2z" />
              </svg>
              <span>GitHub</span>
            </a>
            <a
              href="#quickstart"
              className="px-3 py-1.5 text-xs font-mono font-medium text-black bg-white hover:bg-zinc-200 rounded transition-colors"
            >
              Get Started
            </a>
          </div>

          <button
            onClick={() => setMobileMenuOpen(!mobileMenuOpen)}
            className="md:hidden text-zinc-400 hover:text-white"
            aria-label="Toggle navigation menu"
          >
            {mobileMenuOpen ? <X className="w-5 h-5" /> : <Menu className="w-5 h-5" />}
          </button>
        </div>

        {mobileMenuOpen && (
          <div className="md:hidden border-b border-zinc-800 bg-zinc-950 px-6 py-4 flex flex-col space-y-3 font-mono text-xs text-zinc-400">
            <a href="#what-it-is" onClick={() => setMobileMenuOpen(false)} className="hover:text-white">Concept</a>
            <a href="#how-it-works" onClick={() => setMobileMenuOpen(false)} className="hover:text-white">Pipeline</a>
            <a href="#architecture" onClick={() => setMobileMenuOpen(false)} className="hover:text-white">Architecture</a>
            <a href="#use-cases" onClick={() => setMobileMenuOpen(false)} className="hover:text-white">Use Cases</a>
            <a href="#adapters" onClick={() => setMobileMenuOpen(false)} className="hover:text-white">Adapters</a>
            <a href="#quickstart" onClick={() => setMobileMenuOpen(false)} className="hover:text-white">Docs &amp; Quickstart</a>
            <a href={REPO_URL} target="_blank" rel="noreferrer" className="text-zinc-200 flex items-center space-x-1 pt-2 border-t border-zinc-800">
              <span>View Repository on GitHub</span>
              <ExternalLink className="w-3.5 h-3.5" />
            </a>
          </div>
        )}
      </header>

      {/* 2. Hero Section */}
      <section className="border-b border-zinc-800/80 py-24 sm:py-32 px-6">
        <div className="max-w-4xl mx-auto text-center">
          <div className="inline-flex items-center space-x-2 px-3 py-1 mb-8 border border-zinc-800 bg-zinc-950 rounded text-xs font-mono text-zinc-400">
            <span className="w-1.5 h-1.5 bg-zinc-400 rounded-full"></span>
            <span>Go &amp; PostgreSQL Backend Boilerplate</span>
          </div>

          <h1 className="text-4xl sm:text-5xl lg:text-6xl font-semibold tracking-tight text-white max-w-3xl mx-auto leading-[1.12]">
            Build automations once.<br />Connect anything.
          </h1>

          <p className="mt-6 text-base sm:text-lg text-zinc-400 max-w-2xl mx-auto leading-relaxed">
            A lightweight, extensible automation engine for moving events between services through reusable source and destination adapters.
          </p>

          <div className="mt-10 flex flex-col sm:flex-row items-center justify-center gap-4">
            <a
              href="#quickstart"
              className="w-full sm:w-auto px-6 py-2.5 text-xs font-mono font-medium text-black bg-white hover:bg-zinc-200 rounded flex items-center justify-center space-x-2 transition-colors"
            >
              <span>Get Started</span>
              <ArrowRight className="w-3.5 h-3.5" />
            </a>
            <a
              href={REPO_URL}
              target="_blank"
              rel="noreferrer"
              className="w-full sm:w-auto px-6 py-2.5 text-xs font-mono text-zinc-300 hover:text-white bg-zinc-950 hover:bg-zinc-900 border border-zinc-800 hover:border-zinc-700 rounded flex items-center justify-center space-x-2 transition-colors"
            >
              <svg className="w-4 h-4 fill-current" viewBox="0 0 24 24">
                <path fillRule="evenodd" clipRule="evenodd" d="M12 2C6.477 2 2 6.484 2 12.017c0 4.425 2.865 8.18 6.839 9.504.5.092.682-.217.682-.483 0-.237-.008-.868-.013-1.703-2.782.605-3.369-1.343-3.369-1.343-.454-1.158-1.11-1.466-1.11-1.466-.908-.62.069-.608.069-.608 1.003.07 1.53 1.032 1.53 1.032.892 1.53 2.341 1.088 2.91.832.092-.647.35-1.088.636-1.338-2.22-.253-4.555-1.113-4.555-4.951 0-1.093.39-1.988 1.029-2.688-.103-.253-.446-1.272.098-2.65 0 0 .84-.27 2.75 1.026A9.564 9.564 0 0112 6.844c.85.004 1.705.115 2.504.337 1.909-1.296 2.747-1.027 2.747-1.027.546 1.379.202 2.398.1 2.651.64.7 1.028 1.595 1.028 2.688 0 3.848-2.339 4.695-4.566 4.943.359.309.678.92.678 1.855 0 1.338-.012 2.419-.012 2.747 0 .268.18.58.688.482A10.019 10.019 0 0022 12.017C22 6.484 17.522 2 12 2z" />
              </svg>
              <span>View on GitHub</span>
            </a>
          </div>

          {/* Minimal technical flow */}
          <div className="mt-16 pt-12 border-t border-zinc-900 flex justify-center">
            <div className="inline-flex flex-wrap items-center justify-center gap-2 sm:gap-3 text-xs font-mono text-zinc-400 bg-zinc-950 px-4 py-2 border border-zinc-800 rounded">
              <span className="text-zinc-200">SOURCE</span>
              <span className="text-zinc-600">→</span>
              <span className="text-zinc-200">EVENT</span>
              <span className="text-zinc-600">→</span>
              <span className="text-zinc-200">ROUTE</span>
              <span className="text-zinc-600">→</span>
              <span className="text-zinc-200">DESTINATION</span>
            </div>
          </div>
        </div>
      </section>

      {/* 3. What it is */}
      <section id="what-it-is" className="border-b border-zinc-800/80 py-20 px-6">
        <div className="max-w-5xl mx-auto">
          <div className="mb-12">
            <span className="text-xs font-mono text-zinc-500 uppercase tracking-wider">Concept</span>
            <h2 className="text-2xl sm:text-3xl font-semibold tracking-tight text-white mt-2">
              An automation engine, not another automation platform.
            </h2>
            <p className="mt-3 text-zinc-400 text-sm sm:text-base max-w-3xl leading-relaxed">
              Instead of hardcoding service logic into monolithic scripts or relying on closed SaaS platforms, the engine provides the foundational infrastructure for event-driven workflows.
            </p>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
            <div className="border border-zinc-800 bg-zinc-950/50 p-6 rounded">
              <div className="w-8 h-8 rounded border border-zinc-800 bg-zinc-900 flex items-center justify-center mb-4 text-zinc-300">
                <Radio className="w-4 h-4" />
              </div>
              <h3 className="text-sm font-semibold text-white mb-2">Ingestion &amp; Normalization</h3>
              <p className="text-xs text-zinc-400 leading-relaxed">
                Accepts external webhooks or polls external APIs. Source adapters normalize heterogeneous payloads into a universal Event model.
              </p>
            </div>

            <div className="border border-zinc-800 bg-zinc-950/50 p-6 rounded">
              <div className="w-8 h-8 rounded border border-zinc-800 bg-zinc-900 flex items-center justify-center mb-4 text-zinc-300">
                <GitBranch className="w-4 h-4" />
              </div>
              <h3 className="text-sm font-semibold text-white mb-2">Rule-Based Routing</h3>
              <p className="text-xs text-zinc-400 leading-relaxed">
                Routes connect sources to destinations with optional JSON-based event filtering. Evaluates which destinations should receive each event.
              </p>
            </div>

            <div className="border border-zinc-800 bg-zinc-950/50 p-6 rounded">
              <div className="w-8 h-8 rounded border border-zinc-800 bg-zinc-900 flex items-center justify-center mb-4 text-zinc-300">
                <ShieldCheck className="w-4 h-4" />
              </div>
              <h3 className="text-sm font-semibold text-white mb-2">Idempotent Execution</h3>
              <p className="text-xs text-zinc-400 leading-relaxed">
                Enforces deduplication at both ingress and delivery stages via PostgreSQL uniqueness constraints. Retrying failed targets never duplicates successful ones.
              </p>
            </div>
          </div>

          <div className="mt-8 border border-zinc-800/80 bg-zinc-950 p-6 rounded flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4">
            <div>
              <h4 className="text-sm font-medium text-white">Pluggable Adapter Model</h4>
              <p className="text-xs text-zinc-400 mt-1">
                The core engine knows nothing about specific platforms like Telegram, Discord, or GitHub. Integrations are isolated plugins.
              </p>
            </div>
            <a
              href="#adapters"
              className="px-3.5 py-1.5 text-xs font-mono text-zinc-200 border border-zinc-700 hover:border-zinc-500 rounded whitespace-nowrap transition-colors"
            >
              Explore Adapter Spec →
            </a>
          </div>
        </div>
      </section>

      {/* 4. How it works */}
      <section id="how-it-works" className="border-b border-zinc-800/80 py-20 px-6">
        <div className="max-w-5xl mx-auto">
          <div className="mb-14 text-center sm:text-left">
            <span className="text-xs font-mono text-zinc-500 uppercase tracking-wider">Pipeline</span>
            <h2 className="text-2xl sm:text-3xl font-semibold tracking-tight text-white mt-2">
              The Six-Stage Pipeline
            </h2>
            <p className="mt-3 text-zinc-400 text-sm max-w-2xl">
              Every external event moves through a deterministic sequence from capture to persistent record.
            </p>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-6 gap-3">
            {[
              {
                step: "01",
                name: "SOURCE",
                icon: Radio,
                desc: "An external service sends a webhook push or is polled by an adapter."
              },
              {
                step: "02",
                name: "INGEST",
                icon: Layers,
                desc: "Raw byte stream and metadata are received by the pipeline."
              },
              {
                step: "03",
                name: "NORMALIZE",
                icon: Cpu,
                desc: "Source adapter translates provider data into the canonical Event struct."
              },
              {
                step: "04",
                name: "ROUTE",
                icon: GitBranch,
                desc: "Routing engine matches active routes and evaluates filter criteria."
              },
              {
                step: "05",
                name: "DELIVER",
                icon: Send,
                desc: "Worker goroutines claim delivery jobs and invoke destination adapters."
              },
              {
                step: "06",
                name: "RECORD",
                icon: Database,
                desc: "Delivery outcome is recorded. Failures schedule exponential backoff retries."
              }
            ].map((item, idx) => {
              const Icon = item.icon;
              return (
                <div key={idx} className="border border-zinc-800 bg-zinc-950 p-4 rounded flex flex-col justify-between hover:border-zinc-700 transition-colors">
                  <div>
                    <div className="flex items-center justify-between text-zinc-500 text-xs font-mono mb-4">
                      <span>{item.step}</span>
                      <Icon className="w-3.5 h-3.5 text-zinc-400" />
                    </div>
                    <div className="text-xs font-mono font-semibold text-white mb-2">{item.name}</div>
                    <p className="text-[11px] text-zinc-400 leading-relaxed">{item.desc}</p>
                  </div>
                </div>
              );
            })}
          </div>
        </div>
      </section>

      {/* 5. Architecture */}
      <section id="architecture" className="border-b border-zinc-800/80 py-20 px-6">
        <div className="max-w-5xl mx-auto">
          <div className="mb-12">
            <span className="text-xs font-mono text-zinc-500 uppercase tracking-wider">Engineering</span>
            <h2 className="text-2xl sm:text-3xl font-semibold tracking-tight text-white mt-2">
              PostgreSQL-Backed, Zero Distributed Queue Overhead
            </h2>
            <p className="mt-3 text-zinc-400 text-sm max-w-3xl leading-relaxed">
              No Redis. No Kafka. Delivery jobs are queued directly in PostgreSQL and claimed atomically with <code className="text-zinc-200">FOR UPDATE SKIP LOCKED</code>.
            </p>
          </div>

          <div className="border border-zinc-800 bg-zinc-950 rounded overflow-hidden">
            <div className="px-4 py-3 border-b border-zinc-800 flex items-center justify-between text-xs font-mono text-zinc-400">
              <div className="flex items-center space-x-2">
                <span className="w-2.5 h-2.5 rounded-full bg-zinc-700"></span>
                <span>system-architecture.txt</span>
              </div>
              <span>Concurrency &amp; Storage Layout</span>
            </div>

            <div className="p-6 font-mono text-xs text-zinc-300 overflow-x-auto leading-relaxed">
              <pre className="text-zinc-300">
{`┌─────────────────────────────────┐
│     EXTERNAL SOURCE ADAPTER     │ (Webhook HTTP push / Poller loop)
└────────────────┬────────────────┘
                 │ Raw payload bytes
                 ▼
┌─────────────────────────────────┐
│       NORMALIZATION LAYER       │ adapter.Normalize() -> *event.Event
└────────────────┬────────────────┘
                 │
                 ├──▶ Persist: events table [UNIQUE: source_id + external_event_id]
                 │
                 ▼
┌─────────────────────────────────┐
│         ROUTING ENGINE          │ Evaluates active routes & JSON filter rules
└────────────────┬────────────────┘
                 │
                 ▼ Enqueue: delivery_jobs [UNIQUE: event_id + destination_id]
┌─────────────────────────────────┐
│        POSTGRESQL QUEUE         │ SELECT ... FOR UPDATE SKIP LOCKED
└────────────────┬────────────────┘
                 │
                 ├──▶ Worker Thread #1 ──▶ Destination A ──▶ [DELIVERED]
                 │
                 └──▶ Worker Thread #2 ──▶ Destination B ──▶ [FAILED -> RETRY]`}
              </pre>
            </div>
          </div>

          <div className="grid grid-cols-1 sm:grid-cols-3 gap-6 mt-6">
            <div className="border border-zinc-800/80 bg-zinc-950/40 p-5 rounded">
              <span className="text-xs font-mono text-zinc-400 uppercase">Ingress Idempotency</span>
              <p className="text-xs text-zinc-300 mt-2 font-mono">UNIQUE(source_id, external_event_id)</p>
              <p className="text-xs text-zinc-500 mt-1 leading-normal">
                Duplicate external events are rejected on database level without creating redundant delivery jobs.
              </p>
            </div>

            <div className="border border-zinc-800/80 bg-zinc-950/40 p-5 rounded">
              <span className="text-xs font-mono text-zinc-400 uppercase">Egress Idempotency</span>
              <p className="text-xs text-zinc-300 mt-2 font-mono">UNIQUE(event_id, destination_id)</p>
              <p className="text-xs text-zinc-500 mt-1 leading-normal">
                Each destination receives exactly one delivery task per event. Retrying one destination never resends to others.
              </p>
            </div>

            <div className="border border-zinc-800/80 bg-zinc-950/40 p-5 rounded">
              <span className="text-xs font-mono text-zinc-400 uppercase">Concurrency Control</span>
              <p className="text-xs text-zinc-300 mt-2 font-mono">FOR UPDATE SKIP LOCKED</p>
              <p className="text-xs text-zinc-500 mt-1 leading-normal">
                Multiple background workers claim jobs concurrently with zero race conditions or double processing.
              </p>
            </div>
          </div>
        </div>
      </section>

      {/* 6. Use Cases */}
      <section id="use-cases" className="border-b border-zinc-800/80 py-20 px-6">
        <div className="max-w-5xl mx-auto">
          <div className="mb-12">
            <span className="text-xs font-mono text-zinc-500 uppercase tracking-wider">Applications</span>
            <h2 className="text-2xl sm:text-3xl font-semibold tracking-tight text-white mt-2">
              Supported by the Adapter Architecture
            </h2>
            <p className="mt-3 text-zinc-400 text-sm max-w-3xl leading-relaxed">
              The engine separates protocol mechanics from routing. Implement a source adapter and a destination adapter to bridge any two platforms.
            </p>
          </div>

          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
            {[
              {
                source: "Telegram",
                dest: "Telegram",
                category: "Message Forwarding",
                status: "Architecture Supported"
              },
              {
                source: "RSS Feed",
                dest: "Telegram",
                category: "Content Ingestion",
                status: "Architecture Supported"
              },
              {
                source: "Webhook",
                dest: "Discord",
                category: "Notification Dispatch",
                status: "Architecture Supported"
              },
              {
                source: "GitHub",
                dest: "Discord",
                category: "Commit & Release Tracker",
                status: "Architecture Supported"
              },
              {
                source: "API Poller",
                dest: "API Webhook",
                category: "Service Synchronization",
                status: "Architecture Supported"
              },
              {
                source: "Database CDC",
                dest: "Webhook Sink",
                category: "Event Streaming",
                status: "Architecture Supported"
              }
            ].map((uc, i) => (
              <div key={i} className="border border-zinc-800 bg-zinc-950 p-5 rounded flex flex-col justify-between">
                <div>
                  <div className="flex items-center justify-between text-[11px] font-mono text-zinc-500 mb-3">
                    <span>{uc.category}</span>
                    <span className="text-zinc-600">Adapter Pattern</span>
                  </div>
                  <div className="text-sm font-mono text-white flex items-center space-x-2">
                    <span className="font-semibold">{uc.source}</span>
                    <span className="text-zinc-600">→</span>
                    <span className="font-semibold">{uc.dest}</span>
                  </div>
                </div>
                <div className="mt-4 pt-3 border-t border-zinc-900 flex items-center justify-between text-[10px] font-mono">
                  <span className="text-zinc-400">{uc.status}</span>
                </div>
              </div>
            ))}
          </div>

          <div className="mt-6 border border-zinc-800/80 bg-zinc-950 p-4 rounded text-xs font-mono text-zinc-400 flex flex-col sm:flex-row sm:items-center justify-between gap-3">
            <div className="flex items-center space-x-2">
              <span className="w-2 h-2 rounded-full bg-zinc-400"></span>
              <span><strong>Built-in Adapters included in repository:</strong> Generic Webhook Source, Mock Source, Webhook Destination, Mock Destination, Stdout Logger.</span>
            </div>
          </div>
        </div>
      </section>

      {/* 7. Quick Start */}
      <section id="quickstart" className="border-b border-zinc-800/80 py-20 px-6">
        <div className="max-w-5xl mx-auto">
          <div className="mb-10">
            <span className="text-xs font-mono text-zinc-500 uppercase tracking-wider">Quickstart</span>
            <h2 className="text-2xl sm:text-3xl font-semibold tracking-tight text-white mt-2">
              Run Locally in Under 30 Seconds
            </h2>
            <p className="mt-3 text-zinc-400 text-sm max-w-3xl leading-relaxed">
              The engine requires zero third-party dependencies outside of PostgreSQL. Start everything using Docker Compose.
            </p>
          </div>

          <div className="border border-zinc-800 bg-zinc-950 rounded overflow-hidden">
            <div className="px-4 py-3 border-b border-zinc-800 flex items-center justify-between text-xs font-mono text-zinc-400">
              <div className="flex items-center space-x-2">
                <Terminal className="w-3.5 h-3.5 text-zinc-400" />
                <span>bash</span>
              </div>
              <button
                onClick={() => copyToClipboard(quickstartCommands, 'quickstart')}
                className="flex items-center space-x-1.5 px-2.5 py-1 text-xs font-mono text-zinc-300 hover:text-white bg-zinc-900 border border-zinc-800 rounded transition-colors"
              >
                {copiedCmd === 'quickstart' ? (
                  <>
                    <Check className="w-3 h-3 text-white" />
                    <span>Copied</span>
                  </>
                ) : (
                  <>
                    <Copy className="w-3 h-3" />
                    <span>Copy</span>
                  </>
                )}
              </button>
            </div>

            <div className="p-6 font-mono text-xs text-zinc-200 overflow-x-auto">
              <pre className="text-zinc-200 leading-loose">
{`# 1. Clone the repository
git clone ${REPO_URL}.git
cd EDA-backend-boilerplate

# 2. Configure environment
cp .env.example .env

# 3. Start PostgreSQL and Engine
docker compose up -d`}
              </pre>
            </div>
          </div>

          <div className="mt-8 grid grid-cols-1 md:grid-cols-2 gap-6">
            <div className="border border-zinc-800 bg-zinc-950 p-5 rounded">
              <div className="flex items-center justify-between text-xs font-mono text-zinc-400 mb-2">
                <span>Check Engine Readiness</span>
                <button
                  onClick={() => copyToClipboard('curl -s http://localhost:8080/ready', 'ready-cmd')}
                  className="hover:text-white"
                >
                  {copiedCmd === 'ready-cmd' ? <Check className="w-3.5 h-3.5 text-white" /> : <Copy className="w-3.5 h-3.5" />}
                </button>
              </div>
              <div className="bg-black p-3 border border-zinc-900 rounded font-mono text-xs text-zinc-300">
                curl -s http://localhost:8080/ready
              </div>
              <p className="text-[11px] text-zinc-500 mt-2 font-mono">
                Returns: &#123; "status": "ready", "registered_sources": ["mock", "webhook"], "registered_destinations": ["mock", "webhook", "stdout"] &#125;
              </p>
            </div>

            <div className="border border-zinc-800 bg-zinc-950 p-5 rounded">
              <div className="flex items-center justify-between text-xs font-mono text-zinc-400 mb-2">
                <span>Run Complete Test Suite</span>
                <button
                  onClick={() => copyToClipboard('go test -count=1 -v ./...', 'test-cmd')}
                  className="hover:text-white"
                >
                  {copiedCmd === 'test-cmd' ? <Check className="w-3.5 h-3.5 text-white" /> : <Copy className="w-3.5 h-3.5" />}
                </button>
              </div>
              <div className="bg-black p-3 border border-zinc-900 rounded font-mono text-xs text-zinc-300">
                go test -count=1 -v ./...
              </div>
              <p className="text-[11px] text-zinc-500 mt-2 font-mono">
                Verifies all 10 core requirements + REST API handlers in &lt; 20ms using in-memory mock stores.
              </p>
            </div>
          </div>
        </div>
      </section>

      {/* 8. Adapter Model */}
      <section id="adapters" className="border-b border-zinc-800/80 py-20 px-6">
        <div className="max-w-5xl mx-auto">
          <div className="mb-10">
            <span className="text-xs font-mono text-zinc-500 uppercase tracking-wider">Extensibility</span>
            <h2 className="text-2xl sm:text-3xl font-semibold tracking-tight text-white mt-2">
              Implement an adapter once. Reuse the engine everywhere.
            </h2>
            <p className="mt-3 text-zinc-400 text-sm max-w-3xl leading-relaxed">
              Adding a new integration requires implementing either <code className="text-zinc-200">SourceAdapter</code> or <code className="text-zinc-200">DestinationAdapter</code> and registering it with the factory. No changes to the core engine are needed.
            </p>
          </div>

          <div className="border border-zinc-800 bg-zinc-950 rounded overflow-hidden">
            <div className="px-4 py-2 border-b border-zinc-800 flex items-center justify-between bg-zinc-950">
              <div className="flex items-center space-x-1 sm:space-x-2 overflow-x-auto text-xs font-mono">
                <button
                  onClick={() => setActiveAdapterTab('source')}
                  className={`px-3 py-1.5 rounded transition-colors ${activeAdapterTab === 'source' ? 'bg-white text-black font-medium' : 'text-zinc-400 hover:text-white'}`}
                >
                  SourceAdapter
                </button>
                <button
                  onClick={() => setActiveAdapterTab('destination')}
                  className={`px-3 py-1.5 rounded transition-colors ${activeAdapterTab === 'destination' ? 'bg-white text-black font-medium' : 'text-zinc-400 hover:text-white'}`}
                >
                  DestinationAdapter
                </button>
                <button
                  onClick={() => setActiveAdapterTab('event')}
                  className={`px-3 py-1.5 rounded transition-colors ${activeAdapterTab === 'event' ? 'bg-white text-black font-medium' : 'text-zinc-400 hover:text-white'}`}
                >
                  Event Struct
                </button>
                <button
                  onClick={() => setActiveAdapterTab('registry')}
                  className={`px-3 py-1.5 rounded transition-colors ${activeAdapterTab === 'registry' ? 'bg-white text-black font-medium' : 'text-zinc-400 hover:text-white'}`}
                >
                  Registry Registration
                </button>
              </div>

              <button
                onClick={() => {
                  const code = activeAdapterTab === 'source' ? sourceAdapterCode
                    : activeAdapterTab === 'destination' ? destinationAdapterCode
                    : activeAdapterTab === 'event' ? eventModelCode
                    : registryCode;
                  copyToClipboard(code, 'tab-code');
                }}
                className="text-zinc-400 hover:text-white p-1"
                title="Copy code snippet"
              >
                {copiedCmd === 'tab-code' ? <Check className="w-3.5 h-3.5 text-white" /> : <Copy className="w-3.5 h-3.5" />}
              </button>
            </div>

            <div className="p-6 font-mono text-xs text-zinc-300 overflow-x-auto leading-relaxed bg-black/60">
              <pre className="text-zinc-300">
                {activeAdapterTab === 'source' && sourceAdapterCode}
                {activeAdapterTab === 'destination' && destinationAdapterCode}
                {activeAdapterTab === 'event' && eventModelCode}
                {activeAdapterTab === 'registry' && registryCode}
              </pre>
            </div>
          </div>

          <div className="mt-8 border border-zinc-800 bg-zinc-950/60 p-6 rounded text-xs leading-relaxed text-zinc-400">
            <h4 className="text-sm font-semibold text-white mb-2">Zero Hardcoded Conditionals</h4>
            <p>
              The core engine does not contain statements such as <code className="text-zinc-300">if adapter == "telegram"</code>. When a route is resolved, the background worker requests an adapter instance from the registry by its registered type name and invokes <code className="text-zinc-300">Deliver(ctx, event)</code>.
            </p>
          </div>
        </div>
      </section>

      {/* 9. Open Source / GitHub */}
      <section className="border-b border-zinc-800/80 py-20 px-6">
        <div className="max-w-5xl mx-auto">
          <div className="border border-zinc-800 bg-zinc-950 p-8 sm:p-12 rounded text-center sm:text-left flex flex-col md:flex-row items-center justify-between gap-8">
            <div className="max-w-2xl">
              <span className="text-xs font-mono text-zinc-500 uppercase tracking-wider">Open Source</span>
              <h2 className="text-2xl sm:text-3xl font-semibold tracking-tight text-white mt-2">
                Built to be extended.
              </h2>
              <p className="mt-3 text-zinc-400 text-sm leading-relaxed">
                Clone the repository, customize adapter logic for your services, and deploy as a standalone background engine. Released under the permissive MIT License.
              </p>
              <div className="mt-6 flex flex-wrap items-center gap-4 text-xs font-mono text-zinc-400">
                <span className="px-2.5 py-1 border border-zinc-800 rounded bg-zinc-900 text-zinc-300">License: MIT</span>
                <span className="px-2.5 py-1 border border-zinc-800 rounded bg-zinc-900 text-zinc-300">Go 1.23+</span>
                <span className="px-2.5 py-1 border border-zinc-800 rounded bg-zinc-900 text-zinc-300">PostgreSQL 16</span>
              </div>
            </div>

            <div className="flex flex-col sm:flex-row md:flex-col gap-3 w-full sm:w-auto">
              <a
                href={REPO_URL}
                target="_blank"
                rel="noreferrer"
                className="px-6 py-2.5 text-xs font-mono font-medium text-black bg-white hover:bg-zinc-200 rounded flex items-center justify-center space-x-2 transition-colors whitespace-nowrap"
              >
                <svg className="w-4 h-4 fill-current" viewBox="0 0 24 24">
                  <path fillRule="evenodd" clipRule="evenodd" d="M12 2C6.477 2 2 6.484 2 12.017c0 4.425 2.865 8.18 6.839 9.504.5.092.682-.217.682-.483 0-.237-.008-.868-.013-1.703-2.782.605-3.369-1.343-3.369-1.343-.454-1.158-1.11-1.466-1.11-1.466-.908-.62.069-.608.069-.608 1.003.07 1.53 1.032 1.53 1.032.892 1.53 2.341 1.088 2.91.832.092-.647.35-1.088.636-1.338-2.22-.253-4.555-1.113-4.555-4.951 0-1.093.39-1.988 1.029-2.688-.103-.253-.446-1.272.098-2.65 0 0 .84-.27 2.75 1.026A9.564 9.564 0 0112 6.844c.85.004 1.705.115 2.504.337 1.909-1.296 2.747-1.027 2.747-1.027.546 1.379.202 2.398.1 2.651.64.7 1.028 1.595 1.028 2.688 0 3.848-2.339 4.695-4.566 4.943.359.309.678.92.678 1.855 0 1.338-.012 2.419-.012 2.747 0 .268.18.58.688.482A10.019 10.019 0 0022 12.017C22 6.484 17.522 2 12 2z" />
                </svg>
                <span>View on GitHub</span>
              </a>
              <a
                href="#quickstart"
                className="px-6 py-2.5 text-xs font-mono text-zinc-300 hover:text-white bg-zinc-900 hover:bg-zinc-800 border border-zinc-800 rounded flex items-center justify-center space-x-2 transition-colors whitespace-nowrap"
              >
                <span>Read Quickstart</span>
                <ChevronRight className="w-3.5 h-3.5" />
              </a>
            </div>
          </div>
        </div>
      </section>

      {/* 10. Minimal Footer */}
      <footer className="py-12 px-6 bg-black text-xs font-mono text-zinc-500">
        <div className="max-w-6xl mx-auto flex flex-col sm:flex-row items-center justify-between gap-6">
          <div className="flex flex-col sm:flex-row items-center space-y-2 sm:space-y-0 sm:space-x-4">
            <span className="text-zinc-300 font-semibold">EDA ENGINE</span>
            <span className="hidden sm:inline text-zinc-700">|</span>
            <span>Minimal event-driven automation boilerplate in Go &amp; PostgreSQL.</span>
          </div>

          <div className="flex items-center space-x-6 text-zinc-400">
            <a href={REPO_URL} target="_blank" rel="noreferrer" className="hover:text-white transition-colors">
              GitHub
            </a>
            <a href="#quickstart" className="hover:text-white transition-colors">
              Docs
            </a>
            <a href={REPO_URL + "/blob/main/LICENSE"} target="_blank" rel="noreferrer" className="hover:text-white transition-colors">
              MIT License
            </a>
            <a href="#" className="hover:text-white transition-colors">
              Top ↑
            </a>
          </div>
        </div>
      </footer>
    </div>
  );
}
