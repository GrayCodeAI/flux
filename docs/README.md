# GraycodeRouter Documentation

Welcome to the GraycodeRouter documentation. This directory contains detailed guides and reference material for the Universal LLM Provider Runtime.

## Documentation Index

### Core Documentation

- **[Architecture](ARCHITECTURE.md)** — System architecture, data flow, and design decisions
- **[Provider Setup Guide](guides/CREDENTIAL-SETUP-FLOW.md)** — How to configure credentials and providers
- **[Dynamic Model Discovery](guides/DYNAMIC-MODEL-DISCOVERY.md)** — Architecture and implementation details for live model discovery

### Quick Links

- **[README](../README.md)** — Project overview and quick start
- **[Contributing Guide](../CONTRIBUTING.md)** — How to contribute to GraycodeRouter
- **[Security Policy](../SECURITY.md)** — Security reporting and best practices
- **[Changelog](../CHANGELOG.md)** — Version history and release notes

### Examples

The [`examples/`](../examples/) directory contains runnable code samples:

- **Basic Chat** — Simple synchronous chat with a single provider
- **Streaming** — Server-sent events streaming with continuation
- **Multi-Provider** — Using fallback chains across multiple providers

## Documentation Structure

```
docs/
├── README.md                          # This file
├── ARCHITECTURE.md                    # System architecture
└── guides/
    ├── CREDENTIAL-SETUP-FLOW.md       # Credential configuration
    └── DYNAMIC-MODEL-DISCOVERY.md     # Model discovery architecture
```

## For Developers

If you're contributing to GraycodeRouter:

1. Read [CONTRIBUTING.md](../CONTRIBUTING.md) for development setup
2. Review [ARCHITECTURE.md](ARCHITECTURE.md) to understand the system
3. Check [AGENTS.md](../AGENTS.md) for AI agent context and conventions
4. Run `make ci` locally before submitting PRs

## For Users

If you're using GraycodeRouter in your application:

1. Start with the [Quick Start](../README.md#quick-start) in the main README
2. Review the [Usage examples](../README.md#usage) for common patterns
3. Check the [examples/](../examples/) directory for complete code samples
4. Read the [Provider Setup Guide](guides/CREDENTIAL-SETUP-FLOW.md) for credential configuration

## API Reference

API documentation is available at:
- **[pkg.go.dev](https://pkg.go.dev/github.com/GrayCodeAI/graycode-router)** — Generated Go documentation
- **[ARCHITECTURE.md](ARCHITECTURE.md)** — Core abstractions and interfaces

## Support

- **Issues**: [GitHub Issues](https://github.com/GrayCodeAI/graycode-router/issues)
- **Discussions**: [GitHub Discussions](https://github.com/GrayCodeAI/graycode-router/discussions)
- **Security**: See [SECURITY.md](../SECURITY.md) for vulnerability reporting
