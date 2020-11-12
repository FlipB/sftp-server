{ lib, pkgs, config, custompkgs, ... }:
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
      #default = [];
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
        assertion = cfg.password == "" && cfg.passwordHashed == "" || (cfg.password != "" && cfg.passwordHashed != "");
        message = "You should specify either passwordHashed or password";
      }
      {
        assertion = cfg.port > 1024 && port <= 65536;
        message = "Port should be between 1024 and 65536";
      }
    ];


    systemd.sockets.sftp-server = mkIf cfg.socketActivate {
      wantedBy = [ "sockets.target" ];
      socketConfig.ListenStream = toString cfg.port;
      socketConfig.BindIPv6Only = "default";
    };

    systemd.services.sftp-server = {
      wantedBy = [ "multi-user.target" ];
      serviceConfig.ExecStartPre = ''${pkgs.runtimeShell} -c 'mkdir ${escapeShellArg cfg.dataDir}/root' '';
      serviceConfig.ExecStart = ''
        ${custompkgs.sftp-server}/bin/server \
          ${ if cfg.socketActivate then "-socket" else "-endpoint ${escapeShellArg cfg.interface}:${toString cfg.port}" } \
          ${ optionalString cfg.socketActivate "-exit" } \
          -user ${escapeShellArg cfg.user} \
          ${ optionalString (cfg.password != "") "-password ${escapeShellArg cfg.password}" } \
          ${ optionalString (cfg.passwordHashed != "") "-passwordHash ${escapeShellArg cfg.passwordHashed}" } \
          -hostkey ${cfg.hostKey}
          -root ${escapeShellArg cfg.dataDir}/root
      '';
      serviceConfig.ExecStartPost = ''${pkgs.runtimeShell} -c 'mv ${escapeShellArg cfg.dataDir}/root ${escapeShellArg cfg.dataDir}/root-$RANDOM' '';
    };
  };
}