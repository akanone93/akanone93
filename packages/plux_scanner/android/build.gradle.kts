// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

// The Android side of plux_scanner (decision B21, ADR-0051): the request for
// ML Kit's unbundled barcode model and the probe that tells Dart whether the
// model is ready. The ZXing-C++ fallback is native code built by
// hook/build.dart, not here.
//
// The host opts in to the unbundled model that mobile_scanner then uses with
// `dev.steenbakker.mobile_scanner.useUnbundled=true` in its
// android/gradle.properties; without it the bundled model is used and the
// probe still reports what Play services holds.

group = "dev.plux.plux_scanner"
version = "1.0"

buildscript {
    val kotlinVersion = "2.4.0"
    repositories {
        google()
        mavenCentral()
    }
    dependencies {
        classpath("com.android.tools.build:gradle:9.1.0")
        classpath("org.jetbrains.kotlin:kotlin-gradle-plugin:$kotlinVersion")
    }
}

allprojects {
    repositories {
        google()
        mavenCentral()
    }
}

plugins {
    id("com.android.library")
}

android {
    namespace = "dev.plux.plux_scanner"
    compileSdk = 36

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    sourceSets {
        getByName("main") {
            java.srcDirs("src/main/kotlin")
        }
    }

    defaultConfig {
        // RT-002: Android 7.0 and later.
        minSdk = 24
    }
}

dependencies {
    // The unbundled barcode model, downloaded through Google Play services
    // (about 200 KB in the app instead of about 2.4 MB). Its Play services
    // base brings the module install API used by PluxScannerPlugin.
    implementation("com.google.android.gms:play-services-mlkit-barcode-scanning:18.3.1")
}

kotlin {
    compilerOptions {
        jvmTarget = org.jetbrains.kotlin.gradle.dsl.JvmTarget.JVM_17
    }
}
