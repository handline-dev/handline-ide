import Network
import XCTest

// The driver `hl sim` talks to: one UI test that serves one JSON request per connection on a loopback port of its own until it is told to
// quit, and says which in its log (HANDLINE-SIM-READY <port>). The test host is a Simulator's own process, so it sees every app's
// accessibility tree and can tap and type in any app through XCTest's public API.
final class Driver: XCTestCase {
    private var listener: NWListener?
    private let done = XCTestExpectation(description: "quit")

    func testServe() throws {
        continueAfterFailure = true
        let params = NWParameters.tcp
        params.requiredInterfaceType = .loopback
        let l = try NWListener(using: params, on: .any)
        l.newConnectionHandler = { [weak self] c in self?.serve(c) }
        l.stateUpdateHandler = { state in
            if case .ready = state, let port = l.port { NSLog("HANDLINE-SIM-READY %d", Int(port.rawValue)) }
        }
        l.start(queue: .main)
        listener = l
        _ = XCTWaiter().wait(for: [done], timeout: 7 * 24 * 3600)
        l.cancel()
    }

    private func serve(_ c: NWConnection) {
        c.start(queue: .main)
        var buffer = Data()
        func more() {
            c.receive(minimumIncompleteLength: 1, maximumLength: 1 << 20) { data, _, finished, error in
                if let data = data { buffer.append(data) }
                if let nl = buffer.firstIndex(of: 10) {
                    let reply = self.handle(Data(buffer[..<nl]))
                    c.send(content: reply + Data([10]), completion: .contentProcessed { _ in c.cancel() })
                } else if finished || error != nil {
                    c.cancel()
                } else {
                    more()
                }
            }
        }
        more()
    }

    private func handle(_ line: Data) -> Data {
        var out: [String: Any]
        do {
            guard let req = try JSONSerialization.jsonObject(with: line) as? [String: Any], let op = req["op"] as? String else {
                throw Failure("a request is a JSON object with an op")
            }
            out = ["ok": true]
            try run(op, req, &out)
        } catch {
            out = ["ok": false, "error": "\(error)"]
        }
        return (try? JSONSerialization.data(withJSONObject: out)) ?? Data("{\"ok\":false}".utf8)
    }

    private struct Failure: Error, CustomStringConvertible {
        let description: String
        init(_ d: String) { description = d }
    }

    private func app(_ req: [String: Any]) -> XCUIApplication {
        XCUIApplication(bundleIdentifier: (req["bundle"] as? String) ?? "com.apple.springboard")
    }

    private func num(_ req: [String: Any], _ key: String) throws -> Double {
        guard let v = req[key] as? Double else { throw Failure("\(key) is a number") }
        return v
    }

    private func point(_ app: XCUIApplication, _ x: Double, _ y: Double) -> XCUICoordinate {
        app.coordinate(withNormalizedOffset: .zero).withOffset(CGVector(dx: x, dy: y))
    }

    private func run(_ op: String, _ req: [String: Any], _ out: inout [String: Any]) throws {
        switch op {
        case "ping":
            break
        case "tree":
            let a = app(req)
            let snap = try a.snapshot()
            out["tree"] = node(snap, 0)
        case "tap":
            point(app(req), try num(req, "x"), try num(req, "y")).tap()
        case "swipe":
            let a = app(req)
            let hold = (req["hold"] as? Double) ?? 0.05
            point(a, try num(req, "x1"), try num(req, "y1")).press(forDuration: hold, thenDragTo: point(a, try num(req, "x2"), try num(req, "y2")))
        case "type":
            guard let text = req["text"] as? String else { throw Failure("text is a string") }
            app(req).typeText(text)
        case "button":
            switch req["name"] as? String {
            case "home": XCUIDevice.shared.press(.home)
            default: throw Failure("button is home (a Simulator has no other hardware button XCTest can press)")
            }
        case "rotate":
            let to: [String: UIDeviceOrientation] = ["portrait": .portrait, "upsidedown": .portraitUpsideDown, "left": .landscapeLeft,
                                                      "right": .landscapeRight]
            guard let o = to[(req["to"] as? String) ?? ""] else { throw Failure("to is portrait, upsidedown, left or right") }
            XCUIDevice.shared.orientation = o
        case "quit":
            done.fulfill()
        default:
            throw Failure("no op \(op)")
        }
    }

    private static let types: [UInt: String] = [
        0: "any", 1: "other", 2: "application", 3: "group", 4: "window", 5: "sheet", 6: "drawer", 7: "alert", 8: "dialog", 9: "button",
        10: "radioButton", 11: "radioGroup", 12: "checkBox", 13: "disclosureTriangle", 14: "popUpButton", 15: "comboBox", 16: "menuButton",
        17: "toolbarButton", 18: "popover", 19: "keyboard", 20: "key", 21: "navigationBar", 22: "tabBar", 23: "tabGroup", 24: "toolbar",
        25: "statusBar", 26: "table", 27: "tableRow", 28: "tableColumn", 29: "outline", 30: "outlineRow", 31: "browser", 32: "collectionView",
        33: "slider", 34: "pageIndicator", 35: "progressIndicator", 36: "activityIndicator", 37: "segmentedControl", 38: "picker",
        39: "pickerWheel", 40: "switch", 41: "toggle", 42: "link", 43: "image", 44: "icon", 45: "searchField", 46: "scrollView",
        47: "scrollBar", 48: "staticText", 49: "textField", 50: "secureTextField", 51: "datePicker", 52: "textView", 53: "menu",
        54: "menuItem", 55: "menuBar", 56: "menuBarItem", 57: "map", 58: "webView", 59: "incrementArrow", 60: "decrementArrow",
        61: "timeline", 62: "ratingIndicator", 63: "valueIndicator", 64: "splitGroup", 65: "splitter", 66: "relevanceIndicator",
        67: "colorWell", 68: "helpTag", 69: "matte", 70: "dockItem", 71: "ruler", 72: "rulerMarker", 73: "grid", 74: "levelIndicator",
        75: "cell", 76: "layoutArea", 77: "layoutItem", 78: "handle", 79: "stepper", 80: "tab", 81: "touchBar", 82: "statusItem",
    ]

    /// One element of the tree: only what an agent reads, frames in points, an empty field left out.
    private func node(_ s: XCUIElementSnapshot, _ depth: Int) -> [String: Any] {
        var n: [String: Any] = ["type": Driver.types[s.elementType.rawValue] ?? "type\(s.elementType.rawValue)"]
        if !s.label.isEmpty { n["label"] = s.label }
        if !s.identifier.isEmpty { n["id"] = s.identifier }
        if let v = s.value, "\(v)" != "" { n["value"] = "\(v)" }
        if let p = s.placeholderValue, !p.isEmpty { n["placeholder"] = p }
        let f = s.frame
        n["frame"] = [Double(f.minX), Double(f.minY), Double(f.width), Double(f.height)]
        if !s.isEnabled { n["enabled"] = false }
        if s.isSelected { n["selected"] = true }
        if depth < 60 {
            let kids = s.children.map { node($0, depth + 1) }
            if !kids.isEmpty { n["children"] = kids }
        }
        return n
    }
}
