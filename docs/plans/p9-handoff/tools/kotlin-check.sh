#!/bin/sh
# Compiles the apps' androidTest Kotlin against minimal Android stubs, without an
# Android SDK. Needs Java; downloads kotlinc 2.1.0 and junit 4.13.2 once into $1.
set -eu
work=${1:-/tmp/kt}
here=$(cd "$(dirname "$0")" && pwd)
root=$(git rev-parse --show-toplevel)
mkdir -p "$work" && cd "$work"
[ -x kotlinc/bin/kotlinc ] || { curl -sSL -o k.zip https://github.com/JetBrains/kotlin/releases/download/v2.1.0/kotlin-compiler-2.1.0.zip && unzip -q k.zip; }
[ -f junit.jar ] || curl -sSL -o junit.jar https://repo1.maven.org/maven2/junit/junit/4.13.2/junit-4.13.2.jar
mkdir -p cls && javac -d cls $(find "$here/kotlin-stubs" -name '*.java')
for a in plux_bank plux_express; do
	./kotlinc/bin/kotlinc -cp cls:junit.jar -d "out-$a" "$root/apps/$a/android/app/src/androidTest/kotlin/dev/plux/$a/AccessibilityTreeTest.kt"
done
echo compiled
