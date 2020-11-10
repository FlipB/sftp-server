# TODOs

- make it so you cannot delete the root folder.
  This probably involves reworking/moving pathSanitize call.

# Running


```
# Run SFTP server for user 'root' password 'toor' and take SSH keys from Stdin
# When you have a single file with both private and public key
cat ssh_host_keys_private_public | go run ./cmd/server -passwordHash d6aa6f8195f195aba1442934e28f20dd7c7ea342dd37cbb1ff422a15962f21e9 -hostkey -
```

```
# Run SFTP server for user 'root' password 'toor' and take SSH keys from Stdin
# When you have split files
cat ssh_host_keys_id_rsa ssh_host_keys_id_rsa.pub | go run ./cmd/server -passwordHash d6aa6f8195f195aba1442934e28f20dd7c7ea342dd37cbb1ff422a15962f21e9 -hostkey -
```

```
# Run SFTP server for user 'root' password 'toor' and take SSH keys from
# ./id_rsa and ./id_rsa.pub
go run ./cmd/server -hostkey ./id_rsa -passwordHash d6aa6f8195f195aba1442934e28f20dd7c7ea342dd37cbb1ff422a15962f21e9
```

# Generating the password hash

```
go run ./cmd/sever -user root -plaintextPassword toor
```

# Generating SSH host keys

```
# NOTE this will create one file with both private and public PEM encoded
# SSH host keys.
go run ./cmd/server -generate > ssh_host_keys_private_public
```