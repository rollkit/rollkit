use std::{env, fs, path::Path};
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

    // Create output directories
    let proto_dir = manifest_dir.join("src/proto");
    fs::create_dir_all(&proto_dir)?;

    // Always generate both versions and keep them checked in
    // This way users don't need to regenerate based on features

    // 1. Generate pure message types (no tonic dependencies)
    let mut prost_config = prost_build::Config::new();
    prost_config.out_dir(&proto_dir);
    // Important: we need to rename the output to avoid conflicts
    prost_config.compile_protos(&proto_files, &[proto_root.as_path()])?;

    // Rename the generated file to messages.rs
    let generated_file = proto_dir.join("evnode.v1.rs");
    let messages_file = proto_dir.join("evnode.v1.messages.rs");
    if generated_file.exists() {
        fs::rename(&generated_file, &messages_file)?;
    }

    // 2. Generate full code with gRPC services (always generate, conditionally include)
    tonic_build::configure()
        .build_server(true)
        .build_client(true)
        .out_dir(&proto_dir)
        .compile(&proto_files, &[proto_root.as_path()])?;

    // Rename to services.rs
    let generated_file_2 = proto_dir.join("evnode.v1.rs");
    let services_file = proto_dir.join("evnode.v1.services.rs");
    if generated_file_2.exists() {
        fs::rename(&generated_file_2, &services_file)?;
    }

    println!("cargo:rerun-if-changed={}", proto_root.display());
    Ok(())
}
