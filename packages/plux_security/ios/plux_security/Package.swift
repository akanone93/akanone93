// swift-tools-version: 5.9
// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import PackageDescription

let package = Package(
    name: "plux_security",
    platforms: [
        .iOS("15.0")
    ],
    products: [
        .library(name: "plux-security", targets: ["plux_security"])
    ],
    dependencies: [
        .package(name: "FlutterFramework", path: "../FlutterFramework")
    ],
    targets: [
        .target(
            name: "plux_security",
            dependencies: [
                .product(name: "FlutterFramework", package: "FlutterFramework")
            ]
        )
    ]
)
