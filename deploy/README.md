# Cupola Linux Deployment

This directory contains deploy-time files for running Cupola as a systemd
service named `cupolad`.

The installer creates:

- `cupolad:cupolad`
- `/opt/cupolad/cupolad`
- `/etc/cupolad/config.yaml`
- `/etc/cupolad/cupolad.env`
- `/var/lib/cupolad`
- `/etc/systemd/system/cupolad.service`

Build and install from the repository root:

```sh
make build
sudo deploy/install.sh
```

Or have the installer build the binary:

```sh
sudo deploy/install.sh --build
```

To install a specific binary:

```sh
sudo deploy/install.sh --binary ./cupola
```

Existing `/etc/cupolad/config.yaml` files are preserved by default. To replace
the config with `deploy/config.yaml`:

```sh
sudo deploy/install.sh --overwrite-config
```

After installation:

```sh
systemctl status cupolad
journalctl -u cupolad -f
```

## Updating from your own machine, without SSHing in

`~/bin/build-and-deploy-service` (not part of this repo) rsyncs this repo to
a remote host, builds it there natively, and restarts the service — run
from the repo root. The binary/unit name (`cupolad`) differs from the `cmd/`
subdirectory (`cupola`), so pass `--cmd`:

```sh
SERVICE_DEPLOY_HOST=your.host build-and-deploy-service --cmd cupola cupolad
```

This only updates the binary on an already-installed service (via
`install.sh`, above) — it doesn't create the user, directories, or the unit
file.
