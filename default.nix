{ pkgs ? import <nixpkgs> { } }:
with pkgs;

pkgs.callPackage ./package.nix { }
