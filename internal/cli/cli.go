package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"kerob/internal/version"
	"kerob/pkg/sdk"
)

// App manages the CLI runtime context.
type App struct {
	Out    io.Writer
	ErrOut io.Writer
	Config *Config
}

// NewApp initializes an App instance.
func NewApp() (*App, error) {
	cfg, err := LoadConfig()
	if err != nil {
		return nil, err
	}
	return &App{
		Out:    os.Stdout,
		ErrOut: os.Stderr,
		Config: cfg,
	}, nil
}

// Run parses arguments and executes the requested command. It returns the process exit code.
func Run(args []string) int {
	app, err := NewApp()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Configuration error: %s\n", err.Error())
		return 1
	}
	return app.Execute(args)
}

// Execute runs the CLI logic against provided arguments.
func (a *App) Execute(args []string) int {
	if len(args) == 0 {
		a.printHelp()
		return 0
	}

	// 1. Extract global flags anywhere in args
	var filtered []string
	jsonMode := a.Config.JSON
	endpoint := a.Config.Endpoint
	token := a.Config.Token
	tenant := a.Config.TenantID

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--json":
			jsonMode = true
		case arg == "--endpoint" && i+1 < len(args):
			endpoint = args[i+1]
			i++
		case strings.HasPrefix(arg, "--endpoint="):
			endpoint = strings.TrimPrefix(arg, "--endpoint=")
		case arg == "--token" && i+1 < len(args):
			token = args[i+1]
			i++
		case strings.HasPrefix(arg, "--token="):
			token = strings.TrimPrefix(arg, "--token=")
		case arg == "--tenant" && i+1 < len(args):
			tenant = args[i+1]
			i++
		case strings.HasPrefix(arg, "--tenant="):
			tenant = strings.TrimPrefix(arg, "--tenant=")
		default:
			filtered = append(filtered, arg)
		}
	}

	formatter := NewFormatter(a.Out, a.ErrOut, jsonMode)

	if len(filtered) == 0 || filtered[0] == "-h" || filtered[0] == "--help" || filtered[0] == "help" {
		a.printHelp()
		return 0
	}

	cmd := filtered[0]
	cmdArgs := filtered[1:]

	// Version command does not require client
	if cmd == "version" || cmd == "-v" || cmd == "--version" {
		if jsonMode {
			_ = formatter.PrintJSON(map[string]string{
				"version":    version.Version,
				"commit":     version.Commit,
				"build_date": version.BuildDate,
			})
			return 0
		}
		_, _ = fmt.Fprintln(a.Out, version.Info())
		return 0
	}

	// Shell completion command does not require client
	if cmd == "completion" {
		shell := "bash"
		if len(cmdArgs) > 0 {
			shell = cmdArgs[0]
		}
		if err := GenerateCompletion(a.Out, shell); err != nil {
			formatter.PrintError("%s", err.Error())
			return 1
		}
		return 0
	}

	// Login command uses config directly
	if cmd == "login" {
		ctx := context.Background()
		if err := runLogin(ctx, a.Config, formatter, cmdArgs); err != nil {
			return 1
		}
		return 0
	}

	// Initialize typed SDK Client for network commands
	client, err := sdk.NewClient(endpoint, sdk.WithToken(token), sdk.WithTenant(tenant))
	if err != nil {
		formatter.PrintError("Client initialization failed: %s", err.Error())
		return 1
	}

	ctx := context.Background()

	switch cmd {
	case "tenant":
		if err := runTenant(ctx, client, a.Config, formatter, cmdArgs); err != nil {
			formatter.PrintError("%s", err.Error())
			return 1
		}
		return 0

	case "workflow":
		if err := runWorkflow(ctx, client, formatter, cmdArgs); err != nil {
			formatter.PrintError("%s", err.Error())
			return 1
		}
		return 0

	case "deploy":
		// Convenience alias for 'kerob workflow deploy'
		if err := runWorkflow(ctx, client, formatter, append([]string{"deploy"}, cmdArgs...)); err != nil {
			formatter.PrintError("%s", err.Error())
			return 1
		}
		return 0

	case "event":
		if err := runEvent(ctx, client, formatter, cmdArgs); err != nil {
			formatter.PrintError("%s", err.Error())
			return 1
		}
		return 0

	case "execution":
		if err := runExecution(ctx, client, formatter, cmdArgs); err != nil {
			formatter.PrintError("%s", err.Error())
			return 1
		}
		return 0

	case "logs":
		if err := runLogs(ctx, client, formatter, cmdArgs); err != nil {
			formatter.PrintError("%s", err.Error())
			return 1
		}
		return 0

	case "dlq":
		if err := runDLQ(ctx, client, formatter, cmdArgs); err != nil {
			formatter.PrintError("%s", err.Error())
			return 1
		}
		return 0

	default:
		formatter.PrintError("Unknown command %q. Run 'kerob --help' for available commands.", cmd)
		return 1
	}
}

func (a *App) printHelp() {
	helpText := `Kerob CLI - Developer Tooling & GitOps Automation

USAGE:
  kerob <command> [subcommand] [flags]

COMMANDS:
  version                 Display CLI version and build metadata
  login                   Configure credentials and default endpoint
  tenant                  Inspect and select organizational tenants (list, select, current)
  workflow                Manage workflows (list, get, validate, import, export, deploy, publish, rollback, diff)
  deploy <path>           GitOps deployment with diff, dry-run, validation, and rollback support
  event                   Inspect and replay pipeline events (list, get, replay)
  execution               Inspect and operate execution runs (list, get, retry, pause, resume, cancel, compensate)
  logs <execution_id>     View granular step execution audit logs and output
  dlq                     Dead-letter queue inspection and retries (list, retry, discard)
  completion <shell>      Generate shell completion script (bash, zsh, fish)

GLOBAL FLAGS:
  --json                  Enable script-friendly JSON output mode
  --endpoint <url>        Kerob server endpoint URL (default: http://localhost:8080 or KEROB_ENDPOINT)
  --token <token>         API Bearer token (or KEROB_TOKEN)
  --tenant <tenant_id>    Tenant ID context override (or KEROB_TENANT_ID)
  -h, --help              Show help information

EXAMPLES:
  # Authenticate with a Kerob instance
  kerob login --endpoint http://localhost:8080 --token krb_live_sec_123

  # Validate workflow file and scan for hardcoded secrets
  kerob workflow validate ./workflows/payment.yaml

  # GitOps dry-run deployment comparing local repository with server
  kerob deploy ./workflows --dry-run --diff

  # Apply GitOps workflow deployment
  kerob deploy ./workflows

  # Inspect execution runs and view granular step timeline
  kerob execution list --status failed
  kerob logs 55fa6211-d0bd-4f6d-9850-d3c230682813

  # Replay an event or retry dead-letter record
  kerob event replay ev_1001 --bypass-idempotency
  kerob dlq retry dlq_2002
`
	_, _ = fmt.Fprint(a.Out, helpText)
}
