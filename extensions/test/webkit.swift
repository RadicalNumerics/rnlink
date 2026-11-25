import Foundation
import WebKit
let input = URL(fileURLWithPath: CommandLine.arguments[1])
let rules = try JSONSerialization.jsonObject(with: Data(contentsOf: input)) as! [[String:Any]]
let cache = URL(fileURLWithPath: FileManager.default.currentDirectoryPath + "/extensions/.cache/webkit")
try FileManager.default.createDirectory(at: cache, withIntermediateDirectories: true)
let store = WKContentRuleListStore(url: cache)!
var remaining = rules.count
var failures = 0
for rule in rules {
    let id = rule["id"] as! Int
    let condition = rule["condition"] as! [String:Any]
    let trigger: [String:Any] = ["url-filter": condition["regexFilter"]!, "url-filter-is-case-sensitive": true]
    let payload = [["trigger": trigger, "action": ["type":"block"]]]
    let json = String(data: try JSONSerialization.data(withJSONObject: payload), encoding: .utf8)!
    store.compileContentRuleList(forIdentifier: "rn-test-\(id)", encodedContentRuleList: json) { _, error in
        if let error = error { print("Rule \(id): FAIL \(error.localizedDescription)"); failures += 1 }
        else { print("Rule \(id): PASS") }
        remaining -= 1
    }
}
let deadline = Date().addingTimeInterval(30)
while remaining > 0 && Date() < deadline { RunLoop.current.run(until: Date().addingTimeInterval(0.1)) }
if remaining > 0 { print("TIMEOUT"); exit(2) }
exit(failures > 0 ? 1 : 0)
