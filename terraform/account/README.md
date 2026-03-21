# terraform/account

Account-level hardening: dedicated Terraform deploy role via AssumeRole.

## What this creates

- **`bonfire-base`** — IAM user whose sole permission is `sts:AssumeRole` targeting
  `bonfire-deploy-role`. Long-lived access keys live only in `~/.aws/credentials`.
- **`bonfire-deploy-role`** — IAM role with `PowerUserAccess` + a permission boundary
  that blocks IAM escalation vectors (cannot create users/roles/policies or pass roles).
- **Trust policy** — `bonfire-base` is the only principal allowed to assume the deploy role.

## Apply

```bash
cd terraform/account
terraform init
terraform apply
```

> **Bootstrap note:** The first apply must run with root credentials (or an existing
> admin account), because `bonfire-base` does not yet exist. After apply, create access
> keys for `bonfire-base` and rotate to the new profile.

## Post-apply local config (manual)

After Terraform creates the resources, configure your local AWS CLI:

**`~/.aws/credentials`**
```ini
[bonfire-base]
aws_access_key_id     = <create in IAM console for bonfire-base>
aws_secret_access_key = <secret>
```

**`~/.aws/config`**
```ini
[profile bonfire-deploy]
role_arn       = <deploy_role_arn output>
source_profile = bonfire-base
region         = eu-north-1
```

Then use `AWS_PROFILE=bonfire-deploy terraform ...` (or `--profile bonfire-deploy`)
for all subsequent Terraform runs.

## Root lockdown (manual, outside Terraform)

After confirming the deploy profile works end-to-end:

1. Enable MFA on the root account (AWS Console → Security credentials).
2. Delete root account access keys.

## Permission boundary rationale

`bonfire-deploy-role` carries `PowerUserAccess` but is constrained by
`bonfire-deploy-permission-boundary`, which blocks all IAM write actions and
`sts:PassRole`. This means:

- Terraform can provision EC2, S3, ECS, etc. without restriction.
- Terraform **cannot** create new IAM users/roles or attach broad policies,
  preventing privilege escalation via the deploy credential.
