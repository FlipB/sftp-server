{ stdenv, buildGoModule, fetchFromGitHub }:

# Assert go version
#assert lib.versionAtLeast go.version "1.13";

buildGoModule rec {
  name = "sftp-server";
  pname = "sftp-server";
  # version = "v1.0.0";
  # rev = "v${version}";
  rev = "master";

  src = ./.;

  # The hash of the output of the intermediate fetcher derivation
  vendorSha256 = "0gdfamxg4b6xb2b3d9ri48d1iskj7zfiwsipbmf954mdcqdiqnqb";

  # runVend runs the vend command to generate the vendor directory. This is useful if your code depends on c code and go mod tidy does not include the needed sources to build. 
  runVend = false;

  # Child packages to build
  subPackages = ["./cmd/server"];

  preBuild = ''
    export CGO_ENABLED=0
  '';

  meta = with stdenv.lib; {
    description = "SFTP Server implementation";
    homepage = "https://github.com/flipb/sftp-server";
    platforms = platforms.linux;
    
    license = licenses.mit;
    maintainers = with maintainers; [ flipb ];
  };
}

