use std::{env, path::Path};
use walkdir::WalkDir;

fn main() -> Result<(), Box<dyn std::error::Error>> {
    let manifest_dir_str = env::var("CARGO_MANIFEST_DIR")?;
    let manifest_dir = Path::new(&manifest_dir_str);
    // Make the include dir absolute and resolved (no "..", symlinks, etc.)
    let proto_root = manifest_dir.join("../../../proto").canonicalize()?;

    // Collect the .proto files
    let proto_files: Vec<_> = WalkDir::new(&proto_root)
        .into_iter()
        .filter_map(|e| {
            let p = e.ok()?.into_path();
            (p.extension()?.to_str()? == "proto").then_some(p)
        })
        .collect();

    tonic_build::configure()
        .build_server(true)
        .build_client(true)
        .out_dir("src/proto")
        // pass the absolute include dir
        .compile(&proto_files, &[proto_root.as_path()])?;

    println!("cargo:rerun-if-changed={}", proto_root.display());
    Ok(())
}
