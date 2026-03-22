# terraform/account

Account-level bootstrap: IAM permission boundary, deploy role, admin role,
and minimal base user. This stack must be applied manually — neither
`bonfire-deploy` nor `bonfire-admin` can apply their own stack on first run
(see [Chicken-and-egg problem](#chicken-and-egg-problem) below).

## What this stack creates

- **`bonfire-deploy-permission-boundary`** — IAM policy attached as a permission
  boundary to `bonfire-deploy-role`. Allows PowerUser-level actions but blocks
  **all** `iam:*`, `organizations:*`, and `account:*` operations. The deploy
  role cannot touch IAM at all — account-level changes go through
  `bonfire-admin-role` instead.

- **`bonfire-deploy-role`** — IAM role assumed by Terraform for all routine
  infrastructure stacks (`terraform/bot/`, `terraform/games/*`, etc.). Carries
  `PowerUserAccess` constrained by the permission boundary. The trust policy
  allows only `bonfire-base` to assume it. No MFA required (used frequently by
  CI/automation).

- **`bonfire-admin-role`** — IAM role assumed by Terraform for account-level
  changes only. Carries `AdministratorAccess` (full IAM). **Requires MFA** to
  assume — the trust policy enforces `aws:MultiFactorAuthPresent: true`. Used
  rarely: only when modifying the permission boundary, deploy role, or admin
  role itself.

- **`bonfire-base`** — IAM user whose sole permissions are `sts:AssumeRole`
  targeting `bonfire-deploy-role` and `bonfire-admin-role`. Long-lived access
  keys for this user live in `~/.aws/credentials` as `[bonfire-base]`.

## Two-role model

| Role | Used for | IAM access | MFA required |
|------|----------|-----------|--------------|
| `bonfire-deploy-role` | All routine stacks | None (blocked by boundary) | No |
| `bonfire-admin-role` | `terraform/account/` only | Full (`AdministratorAccess`) | Yes |

Use `bonfire-deploy` for day-to-day Terraform work. Switch to `bonfire-admin`
only when you need to update the permission boundary, the deploy role, or the
admin role itself.

## When to apply this stack

Apply this stack:

- **First-time account setup** — before any other Terraform stack can run.
- **Any change to the permission boundary** (`bonfire-deploy-permission-boundary`).
- **Any change to bonfire-deploy-role** — trust policy, attached policies, or the role itself.
- **Any change to bonfire-admin-role** — trust policy, attached policies, or the role itself.

All other stacks (`terraform/bot/`, `terraform/games/*`, etc.) use the
`bonfire-deploy` profile and do not require elevated credentials.

## Chicken-and-egg problem

Neither role can apply this stack because:

1. On first apply, neither role exists yet.
2. Even after they exist, changes to the permission boundary or either role
   require IAM permissions that `bonfire-deploy` explicitly cannot use, and
   `bonfire-admin` cannot modify its own trust policy.

This is intentional: the roles must never be able to modify their own
constraints without external credentials.

## How to apply (initial bootstrap and updates)

### Option 1: bonfire-admin profile (preferred after first apply)

Once `bonfire-admin-role` exists and you have MFA configured:

```bash
AWS_PROFILE=bonfire-admin terraform apply
```

This requires an MFA token — your AWS SDK/CLI will prompt for it automatically
if `mfa_serial` is set in your `~/.aws/config` (see [After applying](#after-applying)).

### Option 2: AWS root account (first-time bootstrap only)

```bash
export AWS_ACCESS_KEY_ID=<root-key>
export AWS_SECRET_ACCESS_KEY=<root-secret>
cd terraform/account
terraform init
terraform apply
unset AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY
```

> Delete the root access key immediately after use. Enable MFA on root if not
> already done (see [Root lockdown](#root-lockdown)).

### Option 3: Temporary admin IAM user (alternative bootstrap)

1. Create a temporary IAM user with `AdministratorAccess` in the console.
2. Generate access keys for the temporary user.
3. Run `terraform apply` with those credentials.
4. Delete the temporary user and its keys immediately after.

```bash
export AWS_ACCESS_KEY_ID=<temp-admin-key>
export AWS_SECRET_ACCESS_KEY=<temp-admin-secret>
cd terraform/account
terraform init
terraform apply
unset AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY
# Delete the temp user in the IAM console
```

## After applying

**1. Create access keys for bonfire-base** in the IAM console (Users → bonfire-base →
Security credentials → Create access key).

**2. Configure `~/.aws/credentials`:**
```ini
[bonfire-base]
aws_access_key_id     = <key-id from step 1>
aws_secret_access_key = <secret from step 1>
```

**3. Configure `~/.aws/config`:**
```ini
[profile bonfire-deploy]
role_arn       = <deploy_role_arn output from terraform>
source_profile = bonfire-base
region         = eu-north-1

[profile bonfire-admin]
role_arn       = <admin_role_arn output from terraform>
source_profile = bonfire-base
mfa_serial     = arn:aws:iam::<account-id>:mfa/bonfire-base
region         = eu-north-1
```

Get the role ARNs from Terraform output:
```bash
terraform output deploy_role_arn
terraform output admin_role_arn
```

**4. Verify the deploy profile works:**
```bash
aws sts get-caller-identity --profile bonfire-deploy
```

All routine Terraform runs use `AWS_PROFILE=bonfire-deploy`:
```bash
AWS_PROFILE=bonfire-deploy terraform apply
```

**5. Verify the admin profile works (requires MFA):**
```bash
aws sts get-caller-identity --profile bonfire-admin
# You will be prompted for your MFA token
```

## Root lockdown

After confirming both profiles work end-to-end:

1. Enable MFA on the root account (AWS Console → Security credentials → MFA).
2. Delete root account access keys if any exist.

## IAM security model

### bonfire-deploy: no IAM access

`bonfire-deploy-role` carries `PowerUserAccess` but its permission boundary
blocks `iam:*` entirely. Even if a policy attached to the deploy role tries to
grant IAM write access, the effective permissions are the intersection of the
identity policy and the boundary — IAM is always denied.

This means the deploy role cannot create roles, attach policies, or escalate
privileges in any way. Lambda execution roles, ECS task roles, and EC2 instance
profiles must be managed through `bonfire-admin-role` or pre-created outside
Terraform. This is a deliberate trade-off: if a routine stack needs IAM
resources, define them in `terraform/account/` instead.

### bonfire-admin: full IAM, MFA-gated

`bonfire-admin-role` carries `AdministratorAccess` with no permission boundary.
The protection comes from the assume-role trust policy: MFA must be present.
This limits usage to interactive sessions where the operator explicitly provides
an MFA token — automated CI pipelines cannot assume this role.

Use this role sparingly. Each use should be intentional and short-lived.
