# SPDX-FileCopyrightText: 2026 Plux contributors
# SPDX-License-Identifier: Apache-2.0
#
# The iOS side of plux_security (SEC-070, ADR-0057): Plux's own checks for
# a jailbroken, hooked, debugged or repackaged device. System frameworks only.
Pod::Spec.new do |s|
  s.name             = 'plux_security'
  s.version          = '0.1.0'
  s.summary          = 'Runtime application self-protection for Plux apps.'
  s.description      = 'Plux\'s own detections of a hostile iOS environment.'
  s.homepage         = 'https://github.com/nightCode42/plux3'
  s.license          = { :type => 'Apache-2.0' }
  s.author           = { 'Plux contributors' => 'https://github.com/nightCode42/plux3' }
  s.source           = { :path => '.' }
  s.source_files     = 'plux_security/Sources/plux_security/**/*.swift'
  s.dependency 'Flutter'
  # RT-002: iOS 15 and later.
  s.platform = :ios, '15.0'
  s.pod_target_xcconfig = { 'DEFINES_MODULE' => 'YES', 'EXCLUDED_ARCHS[sdk=iphonesimulator*]' => 'i386' }
  s.swift_version = '5.0'
end
