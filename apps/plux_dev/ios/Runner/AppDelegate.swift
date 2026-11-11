import Flutter
import UIKit

@main
@objc class AppDelegate: FlutterAppDelegate, FlutterImplicitEngineDelegate {
  override func application(
    _ application: UIApplication,
    didFinishLaunchingWithOptions launchOptions: [UIApplication.LaunchOptionsKey: Any]?
  ) -> Bool {
    return super.application(application, didFinishLaunchingWithOptions: launchOptions)
  }

  func didInitializeImplicitFlutterEngine(_ engineBridge: FlutterImplicitEngineBridge) {
    GeneratedPluginRegistrant.register(with: engineBridge.pluginRegistry)
    // The phone's name, which Studio shows in the person's list of phones
    // (ADR-0104).
    if let registrar = engineBridge.pluginRegistry.registrar(forPlugin: "PluxDevDevice") {
      let channel = FlutterMethodChannel(name: "dev.plux.dev/device", binaryMessenger: registrar.messenger())
      channel.setMethodCallHandler { call, result in
        if call.method == "name" {
          result(UIDevice.current.name)
        } else {
          result(FlutterMethodNotImplemented)
        }
      }
    }
  }
}
