{
  description = "A development environment with specified tools";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs = { self, nixpkgs }:
    let
      system = "x86_64-linux";
      pkgs = import nixpkgs { inherit system; };
    in {
      devShells.${system}.default = pkgs.mkShell {
        packages = with pkgs; [
          tree
          nodejs
          python312
          uv
          unzip
          nodemon
          gh
          deno
          sqlite
          gitleaks
          golangci-lint
          go
          gotools
          go-junit-report
          go-task
          delve
          etcd
        ];
      };
    };
}
