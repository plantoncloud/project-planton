# Terraform Module: Cloudflare R2 Bucket

This directory contains the Terraform module for deploying Cloudflare R2 buckets.

## Overview

The Terraform module provisions Cloudflare R2 buckets with S3-compatible object storage and zero egress fees. R2 is simpler than AWS S3 by design—no versioning, no bucket policies—optimized for storing and serving content efficiently.

## Module Structure

```
iac/tf/
├── README.md        # This file - deployment guide
├── variables.tf     # Input variables
├── provider.tf      # Cloudflare provider configuration
├── locals.tf        # Local variables and computed values
├── main.tf          # Resource definitions
└── outputs.tf       # Output values
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
   ```

3. **Terraform CLI**:
   ```bash
   # macOS
   brew install terraform

   # Linux
   wget https://releases.hashicorp.com/terraform/1.6.0/terraform_1.6.0_linux_amd64.zip
   unzip terraform_1.6.0_linux_amd64.zip
   sudo mv terraform /usr/local/bin/

   # Verify installation
   terraform version
   ```

## Usage

### Step 1: Create Terraform Configuration

Create a `main.tf` file using this module:

```hcl
terraform {
  required_providers {
    cloudflare = {
      source  = "cloudflare/cloudflare"
      version = "~> 5.23"
    }
  }
}

provider "cloudflare" {
  api_token = var.cloudflare_api_token
}

module "r2_bucket" {
  source = "./iac/tf"

  metadata = {
    name = "media-bucket"
    labels = {
      env     = "production"
      team    = "platform"
    }
  }

  spec = {
    bucket_name   = "myapp-media-assets"
    account_id    = "0a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d"  # Your account ID
    location      = "weur"  # Western Europe
    public_access = true
  }
}

output "bucket_name" {
  value = module.r2_bucket.bucket_name
}
```

### Step 2: Initialize Terraform

```bash
cd iac/tf

# Initialize providers and modules
terraform init
```

### Step 3: Plan Deployment

```bash
# Preview changes
terraform plan
```

**Expected Output**:

```
Terraform will perform the following actions:

  # cloudflare_r2_bucket.main will be created
  + resource "cloudflare_r2_bucket" "main" {
      + id         = (known after apply)
      + name       = "myapp-media-assets"
      + account_id = "0a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d"
      + location   = "weur"
    }

Plan: 1 to add, 0 to change, 0 to destroy.
```

### Step 4: Apply Configuration

```bash
# Apply changes
terraform apply

# Auto-approve (for CI/CD)
terraform apply -auto-approve
```

### Step 5: Verify Deployment

```bash
# View outputs
terraform output

# Test bucket access
aws s3 ls --endpoint-url https://<account-id>.r2.cloudflarestorage.com
```

## Input Variables

### Required Variables

#### `metadata` (object)

Metadata for the R2 bucket resource.

```hcl
metadata = {
  name = "media-bucket"  # Required
}
```

#### `spec` (object)

R2 bucket specification.

**Required fields**:

- **`bucket_name`** (string): DNS-compatible bucket name (3-63 characters)
  ```hcl
  bucket_name = "myapp-assets"
  ```

- **`account_id`** (string): Cloudflare account ID (32 hex characters)
  ```hcl
  account_id = "0a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d"
  ```

**Optional fields**:

- **`location`** (string): Primary region for the bucket (location hint) - Default: `auto`
  - `auto` = no hint; Cloudflare selects the optimal region
  - `wnam` = Western North America
  - `enam` = Eastern North America
  - `weur` = Western Europe
  - `eeur` = Eastern Europe
  - `apac` = Asia-Pacific
  - `oc` = Oceania
  ```hcl
  location = "weur"
  ```
  When `auto` (or omitted), the hint is not sent and Cloudflare chooses the region.

- **`public_access`** (bool): Expose the bucket via the managed r2.dev public URL - Default: `false`
  ```hcl
  public_access = true
  ```
  **Note**: enabling `public_access` provisions the managed r2.dev domain; use a custom domain for production traffic.

## Outputs

| Output | Type | Description |
|--------|------|-------------|
| `bucket_name` | string | The name of the R2 bucket |
| `bucket_url` | string | The path-style S3 API URL of the bucket (its jurisdiction's endpoint plus the bucket name) |
| `custom_domain_urls` | list(string) | URLs of the configured custom domains (one per enabled custom domain) |
| `public_url` | string | The managed r2.dev public URL when `public_access` is enabled; empty otherwise |
| `account_id` | string | The owning Cloudflare account (same as `spec.account_id`) |
| `jurisdiction` | string | The bucket's jurisdiction, normalized: `default`, `eu`, `fedramp`, or `us` |
| `s3_endpoint` | string | The S3 API endpoint serving this bucket's jurisdiction -- the only host that does; S3 clients use it with region `auto` |

Access outputs:

```bash
# View all outputs
terraform output

# Get specific output
terraform output bucket_name
```

Use outputs in other modules:

```hcl
module "other_module" {
  source = "./other-module"
  
  bucket_name = module.r2_bucket.bucket_name
}
```

## Examples

### Example 1: Basic Private Bucket

```hcl
module "private_bucket" {
  source = "./iac/tf"

  metadata = {
    name = "app-data"
  }

  spec = {
    bucket_name  = "myapp-private-data"
    account_id   = "0a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d"
    location     = "enam"  # Eastern North America
    public_access = false
  }
}
```

### Example 2: Public CDN Bucket

```hcl
module "cdn_bucket" {
  source = "./iac/tf"

  metadata = {
    name = "cdn-assets"
  }

  spec = {
    bucket_name  = "public-cdn-assets"
    account_id   = "0a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d"
    location     = "weur"  # Western Europe
    public_access = true
  }
}
```

### Example 3: Multi-Region

```hcl
# US bucket
module "bucket_us" {
  source = "./iac/tf"

  metadata = {
    name = "assets-us"
  }

  spec = {
    bucket_name = "myapp-assets-us"
    account_id  = "0a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d"
    location    = "enam"  # Eastern North America
  }
}

# EU bucket
module "bucket_eu" {
  source = "./iac/tf"

  metadata = {
    name = "assets-eu"
  }

  spec = {
    bucket_name = "myapp-assets-eu"
    account_id  = "0a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d"
    location    = "weur"  # Western Europe
  }
}
```

## State Management

### Local State (Development Only)

By default, Terraform stores state locally in `terraform.tfstate`:

```bash
terraform apply  # Creates terraform.tfstate
```

**Warning**: Do not use local state for production. State files contain sensitive data.

### Remote State (Recommended)

#### S3 Backend

```hcl
terraform {
  backend "s3" {
    bucket         = "my-terraform-state"
    key            = "cloudflare/r2-bucket/terraform.tfstate"
    region         = "us-east-1"
    encrypt        = true
    dynamodb_table = "terraform-lock"
  }
}
```

#### Terraform Cloud

```hcl
terraform {
  cloud {
    organization = "my-org"
    workspaces {
      name = "cloudflare-r2-bucket"
    }
  }
}
```

## Updating the Bucket

Modify your Terraform configuration and re-run:

```bash
terraform plan   # Preview changes
terraform apply  # Apply changes
```

### Common Updates

**Change location**:

```hcl
location = "apac"  # Asia-Pacific instead of weur
```

**Enable public access**:

```hcl
public_access = true
```

**Note**: Changing bucket_name requires bucket replacement (destroy + create).

## Destroying the Bucket

```bash
# Preview what will be deleted
terraform plan -destroy

# Confirm and delete all resources
terraform destroy
```

**Warning**: This permanently deletes the bucket and all objects. Ensure data is backed up.

## CI/CD Integration

### GitHub Actions Example

```yaml
name: Deploy R2 Bucket

on:
  push:
    branches: [main]

jobs:
  terraform:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      
      - uses: hashicorp/setup-terraform@v2
        with:
          terraform_version: 1.6.0
      
      - name: Terraform Init
        run: terraform init
        working-directory: iac/tf
        env:
          CLOUDFLARE_API_TOKEN: ${{ secrets.CLOUDFLARE_API_TOKEN }}
      
      - name: Terraform Plan
        run: terraform plan
        working-directory: iac/tf
        env:
          CLOUDFLARE_API_TOKEN: ${{ secrets.CLOUDFLARE_API_TOKEN }}
      
      - name: Terraform Apply
        run: terraform apply -auto-approve
        working-directory: iac/tf
        env:
          CLOUDFLARE_API_TOKEN: ${{ secrets.CLOUDFLARE_API_TOKEN }}
```

## Troubleshooting

### Common Issues

**Issue**: `Error: authentication error - invalid API token`

**Solution**: Verify `CLOUDFLARE_API_TOKEN` environment variable:
```bash
echo $CLOUDFLARE_API_TOKEN
```

---

**Issue**: `Error: bucket already exists`

**Solution**: Bucket names must be unique within your account. Choose a different name.

---

**Issue**: Public access not working

**Solution**: The Terraform provider doesn't yet support toggling r2.dev public URLs. Enable manually via Cloudflare Dashboard.

---

**Issue**: `Error: resource already exists`

**Solution**: Import existing resource:
```bash
terraform import cloudflare_r2_bucket.main <bucket-id>
```

### Debug Mode

Enable detailed logging:

```bash
export TF_LOG=DEBUG
terraform apply
```

### View Resource State

```bash
# List all resources in state
terraform state list

# Show details of a specific resource
terraform state show cloudflare_r2_bucket.main
```

## Best Practices

1. **Use remote state backend** (S3, Terraform Cloud) for production
2. **Store API tokens in secrets** (GitHub Secrets, AWS Secrets Manager)
3. **Separate environments** (dev, staging, prod) using workspaces or separate state files
4. **Enable state locking** (DynamoDB for S3 backend) to prevent concurrent modifications
5. **Review plans carefully** before applying to production
6. **Tag resources** using `metadata.labels` for cost tracking and organization

## Terraform Workspaces

Manage multiple environments with workspaces:

```bash
# Create dev workspace
terraform workspace new dev
terraform apply

# Create prod workspace
terraform workspace new prod
terraform apply

# Switch between workspaces
terraform workspace select dev
terraform workspace select prod
```

Each workspace maintains separate state.

## Validation

Validate configuration before applying:

```bash
# Check syntax
terraform fmt -check

# Validate configuration
terraform validate

# Security scan (requires tfsec)
tfsec .
```

## Limitations

### Public Access

Enabling `public_access` provisions the managed r2.dev domain directly. For production public access, attach one or more custom domains (`custom_domains`), which this module also provisions.
- See: https://developers.cloudflare.com/r2/buckets/public-buckets/

### Versioning

R2 does not support object versioning, so it is not modeled by this component.

## Additional Resources

- [Cloudflare Terraform Provider Docs](https://registry.terraform.io/providers/cloudflare/cloudflare/latest/docs)
- [Cloudflare R2 API Docs](https://developers.cloudflare.com/api/operations/r2-create-bucket)
- [Component README](../../README.md) - User-facing documentation
- [Presets](../../presets/) - Complete usage examples

## Support

For issues or questions:
1. Check [Common Issues](#common-issues) above
2. Review [Component README](../../README.md)
3. Consult Cloudflare and Terraform official documentation

---

**Ready to deploy?** Run `terraform init && terraform apply` to get started!

