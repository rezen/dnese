# dnese
An amigo to help you easily serve dns queries with your answers & see what questions are being asked.

Originally built for dns spoofing for testing native/desktop apps.


## Usage

```sh
dnese --help
dnese --always 127.0.0.1 --port 5300
```

You can also set the configuration options in `$HOME/.dnese.yaml` which is automatically loaded. You can also use `--config` to specify the path of your config.

**Example Config**  
```yaml
---
# always: 127.0.0.1
port: 5300
resolver: 8.8.8.8:53
rules:
  - pattern: .*\.hey\.com\.?$
    address: 127.0.0.1
```