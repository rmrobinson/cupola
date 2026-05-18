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
