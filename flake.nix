{
  description = "Seedrush Flake";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs?ref=nixos-25.11";
  };

  outputs = { self, nixpkgs }: 
  let pkgs = import nixpkgs { system = "x86_64-linux"; }; in
    {

      packages.x86_64-linux.seedrush = with pkgs;
        buildGoModule {
          nativeBuildInputs = [
              pkg-config
          ];

          buildInputs = [
              glibc
              webkitgtk_4_1
              gtk3
          ];

          pname = "seedrush";

          version = "1.0.0";

          src = ./.;

          vendorHash = null;

          runVend = true;

          ldflags = [
              "-w"
              "-s"
          ];

          tags = [
              "production"
              "linux"
              "webkit2_41"
          ];
        };
    };
}
