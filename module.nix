{ lib, pkgs, config, ... }:
with lib;
let
  cfg = config.services.sftp-server;
in {
  options.services.sftp-server = {
    enable = mkEnableOption "sftp server";

    user = mkOption {
      type = types.str;
      default = "root";
    };
    password = mkOption {
      type = types.str;
      default = "";
    };
    passwordHashed = mkOption {
      description = ''
        Username and Password hashed with SHA256.
        Example: `sha256(username + password)`'';
      type = types.str;
      default = "";
    };
    interface = mkOption {
      type = types.str;
      default = "0.0.0.0";
    };
    port = mkOption {
      type = types.int;
      default = 2211;
    };
    socketActivate = mkOption {
      description = "Start service on incoming connections and stop automatically when idle";
      type = types.bool;
      default = true;
    };
    dataDir = mkOption {
      type = types.path;
      default = "/var/spool/sftp";
      example = "/tmp/sftp-server-root";
      description = ''
        Path to be served over sftp
      '';
    };
    hostKey = mkOption {
      type = types.path;
      default = "/tmp";
      example = "/etc/secrets/sftp-server/ssh_host_rsa_key";
      description = ''
        Path to the PEM encoded host key. Specify the private part eg. id_rsa,
        and both id_rsa and id_rsa.pub will be used.
      '';
    };
  };

  config = mkIf cfg.enable {

    assertions = [
      {
        assertion = !(cfg.password == "" && cfg.passwordHashed == "") || !(cfg.password != "" && cfg.passwordHashed != "");
        message = "You should specify either passwordHashed or password";
      }
      {
        assertion = cfg.port > 1024 && cfg.port <= 65536;
        message = "Port should be between 1024 and 65536";
      }
    ];


    systemd.sockets.sftp-server = mkIf cfg.socketActivate {
      wantedBy = [ "sockets.target" ];
      socketConfig.ListenStream = "${cfg.interface}:${toString cfg.port}";
      socketConfig.BindIPv6Only = "default";
    };

    systemd.services.sftp-server = {
      wantedBy = optional (!cfg.socketActivate) "multi-user.target";
      # Sandboxing stuff
      serviceConfig = {
        # Create dynamic user for service
        DynamicUser = "true";
        # Ensure service user has access to path under /run (NOTE: will be deleted on service stop)
        # RuntimeDirectory = "sftp-server";
        # Ensure created path under /var/lib accessable by the service user.
        StateDirectory = "sftp-server";

        # make entire filesystem read-only
        #ProtectSystem = "strict";
        InaccessiblePaths = "/mnt";
        #ReadWritePaths = "/run/sftp-server";
        # chroot service (Requires binary inside new root)
        RootDirectory="/var/lib/sftp-server";
        BindReadOnlyPaths = "${pkg.custompkgs.sftp-server}:/bin";
        BindReadOnlyPaths = "${cfg.hostKey}/bin/hostkey.pem"; # FIXME
        # Enable /proc /sys and /dev in chroot
        MountAPIVFS = "true";
        # Make /proc, /sys, /dev and /etc read-only
        ProtectSystem = "full";
        ProtectHome = "true";

        # general security measures
        NoNewPrivileges = "true";
        PrivateNetwork = optional (!cfg.socketActivate) "true";
      };
      serviceConfig.ExecStartPre = ''${pkgs.runtimeShell} -c 'mkdir ${escapeShellArg cfg.dataDir}/root' '';
      serviceConfig.ExecStart = ''
        ${pkgs.custompkgs.sftp-server}/bin/server \
          ${ if cfg.socketActivate then "-socket" else "-endpoint ${escapeShellArg cfg.interface}:${toString cfg.port}" } \
          ${ optionalString cfg.socketActivate "-exit" } \
          -user ${escapeShellArg cfg.user} \
          ${ optionalString (cfg.password != "") "-plaintextPassword ${escapeShellArg cfg.password}" } \
          ${ optionalString (cfg.passwordHashed != "") "-passwordHash ${escapeShellArg cfg.passwordHashed}" } \
          -hostkey ${escapeShellArg cfg.hostKey} \
          -root ${escapeShellArg cfg.dataDir}/root
      '';
      serviceConfig.ExecStopPost = ''${pkgs.runtimeShell} -c 'mv ${escapeShellArg cfg.dataDir}/root ${escapeShellArg cfg.dataDir}/root-$RANDOM' '';
    };
  };
}
