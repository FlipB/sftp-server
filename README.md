# TODOs

- make it so you cannot delete the root folder.
  This probably involves reworking/moving pathSanitize call.
- finish unit tests

# Build with nix

```sh
nix-build .
./result/bin/server -h
```

# Testing sftp-server (with `go run`)

## Generate a keypair (PEM encoded) to use as SSH Hostkeys on server 

```sh
go run ./cmd/server -generate > keys.pem
```

## Generating a password hash for a user

```sh
go run ./cmd/sever -user root -plaintextPassword toor
```

## Running

```sh
# Run SFTP server for user 'root' password 'toor' and take SSH keys from Stdin
# When you have a single file with both private and public key
cat keys.pem | go run ./cmd/server -user root -passwordHash d6aa6f8195f195aba1442934e28f20dd7c7ea342dd37cbb1ff422a15962f21e9 -hostkey - -endpoint 127.0.0.1:2222
```

```sh
# Run SFTP server for user 'root' password 'toor' and take SSH keys from file `keys.pem`
go run ./cmd/server -hostkey ./keys.pem -passwordHash d6aa6f8195f195aba1442934e28f20dd7c7ea342dd37cbb1ff422a15962f21e9 -endpoint 127.0.0.1:2222
```

Running with systemd socket activtion. Systemd will start the server and pass a socket.
Server will automatically exit after being idle for 10 seconds.

```sh
# Here we're running the compiled server binary (rather than using `go run`)
systemd-socket-activate -l 2211 ./result/bin/server -socket -exit -hostkey ./keys.pem -passwordHash d6aa6f8195f195aba1442934e28f20dd7c7ea342dd37cbb1ff422a15962f21e9
```
