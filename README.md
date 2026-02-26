# KubeVirt Service Provider

## Development

### Prerequisites

For code generation (when modifying OpenAPI specs):
```bash
npm install -g @redocly/cli
```

### OpenAPI Specification

The API is defined using OpenAPI 3.0. The specification files are:

- **Source**: `api/v1alpha1/openapi.source.yaml` - Edit this file for API changes
- **Generated**: `api/v1alpha1/openapi.yaml` - Auto-generated, do not edit directly

The source file references external DCM schemas which are automatically bundled during code generation.

### Code Generation

Generate API code and resolve external references:

```bash
make generate-api
```

This command will:
1. Bundle external OpenAPI references into a single file
2. Generate Go types, server, client, and embedded spec code

### Building

```bash
make build
```

