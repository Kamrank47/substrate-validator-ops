{
  description = "Reproducible Substrate validator operations, IaC, and telemetry toolchain";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = import nixpkgs { inherit system; };
      in
      {
        devShells.default = pkgs.mkShell {
          packages = with pkgs; [
            go
            opentofu
            kubectl
            kustomize
            conftest
            prometheus
            shellcheck
            python3
            python3Packages.pyyaml
          ];

          shellHook = ''
            echo "Substrate Validator Ops shell ready (OpenTofu, kubectl, Conftest, Go, Prometheus, ShellCheck)."
          '';
        };
      });
}
