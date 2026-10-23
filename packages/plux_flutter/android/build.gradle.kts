// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

// The Android side of plux_flutter's platform channel (ADR-0029): storage
// directory, Keystore-protected secrets, the share sheet and permission
// prompts, the biometric prompt and the protection of secure pages, and the
// opt-in background sync (SYN-014). Its AndroidX dependencies are Core, for
// FileProvider, Biometric and WorkManager.

group = "dev.plux.plux_flutter"
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
    namespace = "dev.plux.plux_flutter"
    compileSdk = 36

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    sourceSets {
        getByName("main") {
            java.srcDirs("src/main/kotlin")
        }
        getByName("test") {
            java.srcDirs("src/test/kotlin")
        }
    }

    defaultConfig {
        // RT-002: Android 7.0 and later.
        minSdk = 24
    }

    testOptions {
        unitTests {
            // The scheduler's request is built from WorkManager classes that
            // read android.os.Build; the framework stubs answer defaults.
            isReturnDefaultValues = true
        }
    }
}

dependencies {
    implementation("androidx.core:core:1.13.1")
    // BiometricPrompt for the SCA key unlock, biometricAuth and the
    // inactivity lock (SEC-027, SEC-094, ADR-0056, ADR-0059); brings
    // androidx.fragment.
    implementation("androidx.biometric:biometric:1.1.0")
    // Periodic background sync that stages a release (SYN-014, ADR-0092).
    implementation("androidx.work:work-runtime:2.10.1")
    // Play Integrity standard requests (SEC-003, ADR-0012).
    implementation("com.google.android.play:integrity:1.6.0")
    // SMS User Consent for OtpInput (SEC-090): the user shares one message;
    // no SMS permission is declared.
    implementation("com.google.android.gms:play-services-auth-api-phone:18.1.0")
    // The Cronet API of the pinned HTTP/2 client (SEC-041). cronet_http
    // ships the implementation and these same versions into the app;
    // compileOnly adds nothing to the runtime and must follow its pins.
    compileOnly("com.google.android.gms:play-services-cronet:18.1.1")
    compileOnly("org.chromium.net:cronet-embedded:143.7445.0")

    // The scheduler's unit tests (src/test); never part of the library.
    testImplementation("junit:junit:4.13.2")
}

kotlin {
    compilerOptions {
        jvmTarget = org.jetbrains.kotlin.gradle.dsl.JvmTarget.JVM_17
    }
}
