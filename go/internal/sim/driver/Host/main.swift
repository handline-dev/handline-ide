import UIKit

// The app the driver's tests are hosted by: it shows nothing, the tests drive other apps.
final class Delegate: UIResponder, UIApplicationDelegate {
    var window: UIWindow?
}

UIApplicationMain(CommandLine.argc, CommandLine.unsafeArgv, nil, NSStringFromClass(Delegate.self))
