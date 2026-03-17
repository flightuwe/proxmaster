# GitOps Pull Agent (VM)

Primärer Deploy-Pfad ist Pull-basiert auf der Proxmaster-VM.

## Einmalig aktivieren

```bash
chmod +x /opt/proxmaster/infra/ops/gitops/proxmaster-deploy.sh
cp /opt/proxmaster/infra/ops/gitops/proxmaster-gitops.service /etc/systemd/system/
cp /opt/proxmaster/infra/ops/gitops/proxmaster-gitops.timer /etc/systemd/system/
systemctl daemon-reload
systemctl enable --now proxmaster-gitops.timer
```

## Manuell ausfuehren

```bash
systemctl start proxmaster-gitops.service
journalctl -u proxmaster-gitops.service -n 100 --no-pager
```

## Runtime-Env

Die Variablen kommen aus `/opt/proxmaster/infra/.env`:

- `PROXMASTER_GITOPS_REPO_DIR`
- `PROXMASTER_GITOPS_BRANCH`
- `PROXMASTER_GITOPS_COMPOSE_FILE`
- `PROXMASTER_GITOPS_ENV_FILE`
- `PROXMASTER_GITOPS_HEALTH_URL`
- `PROXMASTER_GITOPS_ROLLBACK_ON_FAIL`

