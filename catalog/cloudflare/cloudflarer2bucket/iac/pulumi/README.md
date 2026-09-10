# Pulumi Module: Cloudflare R2 Bucket

This directory contains the Pulumi module for deploying Cloudflare R2 buckets using Planton.

## Overview

The Pulumi module provisions Cloudflare R2 buckets with S3-compatible object storage and zero egress fees, along with their bucket-scoped configuration: managed and custom-domain access, CORS, lifecycle, and object lock. R2 is simpler than S3—no versioning, no bucket policies—while covering the object-storage surface most teams actually use.

## Directory Structure

```
iac/pulumi/
├── README.md              # This file - deployment guide
├── overview.md            # Architecture and design decisions
├── main.go                # Pulumi entrypoint
├── Pulumi.yaml            # Pulumi project configuration
├── Makefile               # Build and deployment helpers
├── debug.sh               # Debug script for local testing
└── module/
    ├── main.go            # Module entrypoint (Resources function)
    ├── locals.go          # Local variables and helpers
    ├── bucket.go          # Core R2 bucket provisioning logic
    └── outputs.go         # Output constant definitions
```

## Prerequisites

1. **Cloudflare Account**:
   - Active Cloudflare account
   - R2 enabled (free tier available)
   - Cloudflare API token with permissions:
     - R2: Edit
     - Account Settings: Read

2. **Required Environment Variables**:
   ```bash
   export CLOUDFLARE_API_TOKEN="your-api-token-here"
   export CLOUDFLARE_ACCOUNT_ID="your-32-char-account-id"
   ```

3. **Pulumi CLI**:
   ```bash
   # macOS
   brew install pulumi

   # Linux
   curl -fsSL https://get.pulumi.com | sh

   # Verify installation
   pulumi version
   ```

4. **Go SDK**:
   - Go 1.21 or later
   - Pulumi Cloudflare provider SDK (auto-installed)

## Deployment

### Step 1: Create Stack Input

Create a YAML file defining your R2 bucket configuration:

```yaml
# cloudflare-r2-stack-input.yaml
target:
  metadata:
    name: media-bucket
  spec:
    bucketName: myapp-media-assets
    accountId: "0a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d"  # Your account ID
    location: weur  # Western Europe
    publicAccess: true

provider_config:
  # Cloudflare credentials provided via environment variables
```

### Step 2: Initialize Pulumi Stack

```bash
cd iac/pulumi

# Initialize a new stack (first time only)
pulumi stack init dev

# Optional: Set Pulumi config
pulumi config set cloudflare:apiToken --secret "${CLOUDFLARE_API_TOKEN}"
```

### Step 3: Deploy

```bash
# Preview changes
pulumi preview --stack dev

# Apply changes
pulumi up --stack dev
```

**Expected Output**:

```
Updating (dev)

View in Browser (Ctrl+O): https://app.pulumi.com/...

     Type                      Name                Status
 +   pulumi:pulumi:Stack       r2-bucket-dev       created
 +   └─ cloudflare:index:R2Bucket   bucket         created

Outputs:
    bucket_name: "myapp-media-assets"

Resources:
    + 1 created

Duration: 3s
```

### Step 4: Verify Deployment

```bash
# View stack outputs
pulumi stack output

# Test bucket access
aws s3 ls --endpoint-url https://<account-id>.r2.cloudflarestorage.com
```

## Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `CLOUDFLARE_API_TOKEN` | Yes | Cloudflare API token with R2 edit permissions |
| `CLOUDFLARE_ACCOUNT_ID` | Yes | Cloudflare account ID (32 hex characters) |
| `PULUMI_ACCESS_TOKEN` | Optional | Required for Pulumi Cloud backend |

## Stack Outputs

After deployment, the following outputs are available:

- **`bucket_name`**: The name of the created R2 bucket
- **`bucket_url`**: The path-style S3 API URL of the bucket (its jurisdiction's endpoint plus the bucket name)
- **`custom_domain_urls`**: One URL per enabled custom domain
- **`public_url`**: The managed `r2.dev` URL when public access is enabled; empty otherwise
- **`account_id`**: The owning Cloudflare account
- **`jurisdiction`**: The bucket's jurisdiction, normalized (`default`, `eu`, `fedramp`, `us`)
- **`s3_endpoint`**: The S3 API endpoint serving the bucket's jurisdiction -- the only host that does; S3 clients use it with region `auto`

Access outputs:

```bash
# View all outputs
pulumi stack output

# Get specific output
pulumi stack output bucket_name
```

## Updating the Bucket

Modify your stack input YAML and re-run:

```bash
pulumi up --stack dev
```

Pulumi will show a diff of changes before applying.

### Common Updates

**Change location**:
```yaml
location: apac  # Asia-Pacific instead of weur
```

**Enable public access**:
```yaml
publicAccess: true
```

**Note**: Some changes (like bucket name) require bucket replacement (destroy + create).

## Destroying the Bucket

```bash
# Preview what will be deleted
pulumi destroy --stack dev --preview

# Confirm and delete all resources
pulumi destroy --stack dev
```

**Warning**: This permanently deletes the bucket and all objects. Ensure data is backed up.

## Debugging

### Enable Debug Mode

```bash
# Run with verbose logging
pulumi up --stack dev --logtostderr -v=9

# Or use the debug script
./debug.sh
```

### Debug Script

The `debug.sh` script provides detailed logging:

```bash
#!/bin/bash
set -x  # Print commands
export PULUMI_DEBUG_COMMANDS=true
export PULUMI_DEBUG_GRPC=true

pulumi up --stack dev --logtostderr -v=9
```

### Common Issues

**Issue**: `Error: authentication error - invalid API token`

**Solution**: Verify `CLOUDFLARE_API_TOKEN` environment variable is set correctly

---

**Issue**: `Error: account_id is required`

**Solution**: Ensure `account_id` is set in stack input or via environment variable

---

**Issue**: `Error: bucket already exists`

**Solution**: Bucket names must be unique within your account. Choose a different name.

---

**Issue**: Public access warning

**Solution**: The Pulumi provider doesn't yet support toggling r2.dev public URLs directly. Enable manually via Cloudflare Dashboard after creation.

## Multi-Environment Deployment

Use separate Pulumi stacks for dev, staging, and production:

```bash
# Development
pulumi stack init dev
pulumi up --stack dev

# Staging
pulumi stack init staging
pulumi up --stack staging

# Production
pulumi stack init prod
pulumi up --stack prod
```

Each stack maintains independent state.

## State Management

By default, Pulumi stores state in Pulumi Cloud (free for individuals). For self-hosted state:

```bash
# Use S3 backend
pulumi login s3://my-pulumi-state-bucket

# Use local filesystem (not recommended for teams)
pulumi login file://~/.pulumi

# Use Azure Blob
pulumi login azblob://my-container

# Use Google Cloud Storage
pulumi login gs://my-pulumi-state-bucket
```

## CI/CD Integration

### GitHub Actions Example

```yaml
name: Deploy R2 Bucket

on:
  push:
    branches: [main]

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      
      - uses: actions/setup-go@v4
        with:
          go-version: '1.21'
      
      - uses: pulumi/actions@v4
        with:
          command: up
          stack-name: prod
          work-dir: iac/pulumi
        env:
          PULUMI_ACCESS_TOKEN: ${{ secrets.PULUMI_ACCESS_TOKEN }}
          CLOUDFLARE_API_TOKEN: ${{ secrets.CLOUDFLARE_API_TOKEN }}
          CLOUDFLARE_ACCOUNT_ID: ${{ secrets.CLOUDFLARE_ACCOUNT_ID }}
```

## Testing

Run the module tests:

```bash
cd module
go test -v ./...
```

## Troubleshooting

### View Resource Details

```bash
# List all resources in stack
pulumi stack --show-urns

# View resource details
pulumi stack export
```

### Refresh State

If R2 bucket was modified outside of Pulumi:

```bash
pulumi refresh --stack dev
```

### Import Existing Bucket

To import an existing R2 bucket:

```bash
pulumi import cloudflare:index/r2Bucket:R2Bucket main <bucket-id>
```

## Best Practices

1. **Use environment-specific stacks**: Separate dev, staging, prod
2. **Store secrets securely**: Use Pulumi secrets or external vaults
3. **Enable stack tags**: Tag resources for cost allocation
4. **Use remote state backend**: Don't rely on local state files
5. **Implement CI/CD**: Automate deployments via GitHub Actions or similar
6. **Test in dev first**: Always validate changes in non-prod before prod deployment

## Limitations

### Public Access

The managed r2.dev public URL has its own lifecycle and is configured outside this module. For production public access, attach a custom domain (`custom_domain`), which this module provisions directly.

### Versioning

R2 does not support object versioning, so it is not modeled by this component.

## Additional Resources

- [Pulumi Cloudflare Provider Docs](https://www.pulumi.com/registry/packages/cloudflare/)
- [Cloudflare R2 API Docs](https://developers.cloudflare.com/api/operations/r2-create-bucket)
- [Component README](../../README.md) - User-facing component documentation

## Support

For issues or questions:
1. Check [Common Issues](#common-issues) above
2. Review overview.md for architectural context
3. Consult Cloudflare and Pulumi official documentation

---

**Ready to deploy?** Run `pulumi up --stack dev` to get started!

