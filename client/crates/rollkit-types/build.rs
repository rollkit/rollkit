use std::path::PathBuf;
use walkdir::WalkDir;

// Collect all .proto files recursively under /proto/ and compile.
fn main() -> Result<(), Box<dyn std::error::Error>> {
    let proto_files: Vec<PathBuf> = WalkDir::new("../../../proto")
        .into_iter()
        .filter_map(|entry| {
            let path = entry.ok()?.path().to_path_buf();
            if path.extension().is_some_and(|ext| ext == "proto") {
                Some(path)
            } else {
                None
            }
        })
        .collect();

    tonic_build::configure()
        .build_server(true)
        .build_client(true)
        .out_dir("src/proto")
        // Ensure consistent formatting across different protoc versions
        .protoc_arg("--experimental_allow_proto3_optional")
        .compile(&proto_files, &["../../../proto"])?;
    
    // Add a note about the protoc version used
    println!("cargo:warning=Proto files compiled with protoc. For consistent results, use protoc version 25.x");

    Ok(())
}
