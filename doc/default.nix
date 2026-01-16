{
  stdenv,
  nixdoc,
  mdbook,
}:

stdenv.mkDerivation {
  pname = "deltanar-docs-html";
  version = "0.1";
  src = ../.;
  nativeBuildInputs = [
    nixdoc
    mdbook
  ];

  dontConfigure = true;
  dontFixup = true;

  env.RUST_BACKTRACE = 1;

  buildPhase = ''
    runHook preBuild
    cd doc
    mdbook build
    runHook postBuild
  '';

  installPhase = ''
    runHook preInstall
    mv book $out
    runHook postInstall
  '';
}
