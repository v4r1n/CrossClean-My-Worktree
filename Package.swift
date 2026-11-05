// swift-tools-version:5.10
import PackageDescription

let package = Package(
    name: "CleanMyWT",
    platforms: [.macOS(.v14)],
    products: [
        .executable(name: "CleanMyWT", targets: ["CleanMyWT"]),
        .executable(name: "wtscan", targets: ["wtscan"]),
    ],
    targets: [
        .target(name: "WorktreeKit"),
        .executableTarget(name: "CleanMyWT", dependencies: ["WorktreeKit"]),
        .executableTarget(name: "wtscan", dependencies: ["WorktreeKit"]),
    ]
)
