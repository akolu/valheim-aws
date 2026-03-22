# terraform/account

Account-level bootstrap: IAM permission boundary, delegation policy, deploy role,
and minimal base user. This stack must be applied manually — `bonfire-deploy` cannot
apply its own stack (see [Chicken-and-egg problem](#chicken-and-egg-problem) below).

## What this stack creates

- **`bonfire-deploy-permission-boundary`** — IAM policy attached as a permission
  boundary to `bonfire-deploy-role`. Allows PowerUser-level actions but blocks all
  IAM write operations by default. The boundary is what makes the delegation pattern
  safe: any role created by the deploy role inherits the same constraint.

- **`bonfire-deploy-iam-delegation`** — IAM policy that re-grants specific IAM role
  management actions (create, attach, delete, pass) to `bonfire-deploy-role`, but
  only when the same permission boundary is enforced on the target resource. This
  allows Terraform to create Lambda execution roles and EC2 instance profiles without
  opening an escalation path.

- **`bonfire-deploy-role`** — IAM role assumed by Terraform for all other stacks.
  Carries `PowerUserAccess` plus `bonfire-deploy-iam-delegation`, constrained by the
  permission boundary. The trust policy allows only `bonfire-base` to assume it.

- **`bonfire-base`** — IAM user whose sole permission is `sts:AssumeRole` targeting
  `bonfire-deploy-role`. Long-lived access keys for this user live in
  `~/.aws/credentials` as `[bonfire-base]`.

## When to apply this stack

Apply this stack:

- **First-time account setup** — before any other Terraform stack can run.
- **Any change to the permission boundary** (`bonfire-deploy-permission-boundary`).
- **Any change to the deploy role** — trust policy, attached policies, or the role
  itself.
- **Any change to the IAM delegation policy** (`bonfire-deploy-iam-delegation`).

All other stacks (`terraform/bot/`, `terraform/games/*`, etc.) use the
`bonfire-deploy` profile and do not require elevated credentials.

## Chicken-and-egg problem

`bonfire-deploy` cannot apply this stack because:

1. The deploy role is *created* by this stack — it does not exist yet on first apply.
2. Even after the role exists, changes to the permission boundary or the deploy role
   itself require IAM permissions that the boundary explicitly blocks for the deploy
   role.

This is intentional: the deploy role must never be able to modify its own constraints.

## How to apply

You need credentials with `iam:*` on the account stack resources. In order of preference:

### Option 1: Temporarily escalate bonfire-base (preferred for post-bootstrap changes)

If `bonfire-base` already exists and you need to update the stack:

1. In the IAM console, attach an inline policy to `bonfire-base` granting `iam:*`
   on the specific resources this stack manages (or `iam:*` on `*` for a short-lived
   session).
2. Run `terraform apply` using `AWS_PROFILE=bonfire-base`.
3. Remove the inline policy immediately after apply.

This minimises the window of elevated access and avoids using root credentials.

### Option 2: AWS root account

Use the root account access key (or root console session) for the apply:

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

### Option 3: Temporary admin IAM user

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
```

Get the `deploy_role_arn` from Terraform output:
```bash
terraform output deploy_role_arn
```

**4. Verify the profile works:**
```bash
aws sts get-caller-identity --profile bonfire-deploy
```

All subsequent Terraform runs for other stacks use `AWS_PROFILE=bonfire-deploy`:
```bash
AWS_PROFILE=bonfire-deploy terraform apply
```

## Root lockdown

After confirming the deploy profile works end-to-end:

1. Enable MFA on the root account (AWS Console → Security credentials → MFA).
2. Delete root account access keys if any exist.

## IAM escalation protection

### Why `iam:CreateRole` is blocked by default

`bonfire-deploy-role` carries `PowerUserAccess`, which is a broad allow-all policy
with a `NotAction` list excluding IAM and a few other sensitive services. If the role
could freely create IAM roles and attach policies to them, it could create a new admin
role and escalate to full account access — bypassing all intended restrictions.

The permission boundary (`bonfire-deploy-permission-boundary`) enforces a hard ceiling:
even if a policy attached to the deploy role tries to grant IAM write access, the
effective permissions are the intersection of the identity policy and the boundary.
`iam:CreateRole` is in the boundary's `NotAction` block, so it is denied regardless
of what identity policies say.

### The delegation pattern (boundary condition)

The `bonfire-deploy-iam-delegation` policy re-opens a narrow window: the deploy role
*can* create and manage roles, but only when the request includes:

```
Condition:
  StringEquals:
    iam:PermissionsBoundary: arn:aws:iam::<account>:policy/bonfire-deploy-permission-boundary
```

This condition is evaluated by IAM at call time. If Terraform tries to create a role
without enforcing the boundary, the call is denied. If it creates a role with the
boundary enforced, the new role is equally constrained — it can never do more than
the deploy role itself.

The result: the deploy role can provision Lambda execution roles, ECS task roles, and
EC2 instance profiles as part of normal infrastructure management, but it cannot
create a privileged role that escapes the boundary. The constraint is self-propagating.

**Before modifying the boundary policy:** understand that relaxing it affects every
role the deploy credential has ever created, not just the deploy role itself. The
boundary ARN is hard-coded into the delegation condition, so the policy name
`bonfire-deploy-permission-boundary` must not change without updating all dependent
role definitions.
