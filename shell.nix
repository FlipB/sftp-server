with import <nixpkgs> { };

# I use this with Nix Environment Selector in VS Code to run VS Code in a suitable env.
mkShell {
  buildInputs = [
    go_1_15
    vgo2nix
    # (import ./default.nix { inherit pkgs; })
  ];
}
