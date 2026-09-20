import AuthenticationServices
import Capacitor
import WebKit

/// Configures the native viewport and system authentication for the bundled frontend.
class ChattoViewController: CAPBridgeViewController {
    private var launchCover: UIView?
    private var keyboardBottomConstraint: NSLayoutConstraint?

    override func viewDidLoad() {
        super.viewDidLoad()
        guard let webView else { return }

        // Keep a full-window host while UIKit moves the webview's bottom edge
        // with the keyboard. The plugin's delayed frame resizing is disabled.
        let host = UIView(frame: webView.frame)
        host.backgroundColor = UIColor(named: "LaunchBackground")
        view = host
        webView.translatesAutoresizingMaskIntoConstraints = false
        host.addSubview(webView)
        if #available(iOS 17.0, *) {
            // The web page already reserves space for the home indicator.
            host.keyboardLayoutGuide.usesBottomSafeArea = false
        }
        let bottom = webView.bottomAnchor.constraint(equalTo: host.keyboardLayoutGuide.topAnchor)
        keyboardBottomConstraint = bottom
        NSLayoutConstraint.activate([
            webView.topAnchor.constraint(equalTo: host.topAnchor),
            webView.leadingAnchor.constraint(equalTo: host.leadingAnchor),
            webView.trailingAnchor.constraint(equalTo: host.trailingAnchor),
            bottom
        ])
    }

    override func viewDidLayoutSubviews() {
        super.viewDidLayoutSubviews()
        if #unavailable(iOS 17.0) {
            // Older guides rest at the safe-area edge when the keyboard is
            // hidden. Restore the full height without adding a second inset.
            let inset = view.safeAreaInsets.bottom
            let adjustment = view.keyboardLayoutGuide.layoutFrame.height <= inset ? inset : 0
            if keyboardBottomConstraint?.constant != adjustment {
                keyboardBottomConstraint?.constant = adjustment
            }
        }
    }

    override func capacitorDidLoad() {
        // Reuse the OS launch screen until the bundled HTML has had a paint
        // opportunity. This covers WebKit startup without a minimum display time.
        if let webView,
           let cover = UIStoryboard(name: "LaunchScreen", bundle: nil).instantiateInitialViewController()?.view {
            webView.backgroundColor = UIColor(named: "LaunchBackground")
            cover.frame = webView.bounds
            cover.autoresizingMask = [.flexibleWidth, .flexibleHeight]
            webView.addSubview(cover)
            launchCover = cover
            webView.configuration.userContentController.add(LaunchReadyHandler(owner: self), name: "chattoLaunchReady")
            webView.configuration.userContentController.addUserScript(WKUserScript(
                source: """
                // Resolve CSS colours (including OKLCH) to sRGB for UIKit.
                function syncNativeBackground() {
                    const canvas = document.createElement('canvas');
                    canvas.width = canvas.height = 1;
                    const context = canvas.getContext('2d');
                    context.fillStyle = getComputedStyle(document.body).backgroundColor;
                    context.fillRect(0, 0, 1, 1);
                    const rgba = Array.from(context.getImageData(0, 0, 1, 1).data);
                    window.webkit.messageHandlers.chattoLaunchReady.postMessage(rgba);
                }
                requestAnimationFrame(() => requestAnimationFrame(syncNativeBackground));
                window.addEventListener('keyboardWillShow', syncNativeBackground);
                new MutationObserver(syncNativeBackground).observe(document.documentElement, {
                    attributes: true, attributeFilter: ['data-theme', 'style']
                });
                """,
                injectionTime: .atDocumentEnd,
                forMainFrameOnly: true
            ))
        }
        // Capacitor's zoomEnabled=false only cancels pinch gestures. Apply scale
        // limits before the first page loads to also prevent focus and double-tap
        // zoom. Keep this policy in the native shell, separate from browser pages.
        let viewportScript = WKUserScript(
            source: """
            const viewport = document.querySelector('meta[name="viewport"]');
            if (viewport) {
                viewport.content += ', minimum-scale=1, maximum-scale=1, user-scalable=no';
            }
            // The shared layout owns the top inset. Reserve the bottom inset
            // inside the page so the inner background continues behind the home bar.
            const safeAreaStyle = document.createElement('style');
            safeAreaStyle.textContent = `
                :root { --mobile-sidebar-safe-bottom: env(safe-area-inset-bottom, 0px); }
                body { padding-bottom: env(safe-area-inset-bottom, 0px); background-color: var(--color-background); }
                [data-safari-bottom-edge] { display: none; }
            `;
            document.head.appendChild(safeAreaStyle);
            """,
            injectionTime: .atDocumentEnd,
            forMainFrameOnly: true
        )
        webView?.configuration.userContentController.addUserScript(viewportScript)
        bridge?.registerPluginInstance(ChattoAuthorizationPlugin())
    }

    /// The static HTML includes the loading UI; server data is not needed here.
    fileprivate func revealWebContent() {
        guard let cover = launchCover else { return }
        launchCover = nil
        UIView.animate(withDuration: UIAccessibility.isReduceMotionEnabled ? 0 : 0.15, animations: {
            cover.alpha = 0
        }, completion: { _ in cover.removeFromSuperview() })
    }

    /// Paint the window exposed around the keyboard after the webview shrinks.
    fileprivate func updateWindowBackground(_ rgba: [Double]) {
        guard rgba.count == 4, rgba.allSatisfy({ $0.isFinite && $0 >= 0 && $0 <= 255 }) else { return }
        let background = UIColor(
            red: CGFloat(rgba[0] / 255), green: CGFloat(rgba[1] / 255),
            blue: CGFloat(rgba[2] / 255), alpha: CGFloat(rgba[3] / 255)
        )
        view.backgroundColor = background
        view.window?.backgroundColor = background
    }
}

/// Avoid a retain cycle between the webview's message handler and its controller.
private final class LaunchReadyHandler: NSObject, WKScriptMessageHandler {
    weak var owner: ChattoViewController?

    init(owner: ChattoViewController) {
        self.owner = owner
    }

    func userContentController(_ userContentController: WKUserContentController, didReceive message: WKScriptMessage) {
        guard message.frameInfo.isMainFrame,
              message.frameInfo.securityOrigin.protocol == "capacitor",
              message.frameInfo.securityOrigin.host == "localhost" else { return }
        if let rgba = message.body as? [Double] {
            owner?.updateWindowBackground(rgba)
        }
        owner?.revealWebContent()
    }
}

/// Keeps a single, time-bounded authentication session alive until completion.
@objc(ChattoAuthorizationPlugin)
public class ChattoAuthorizationPlugin: CAPPlugin, CAPBridgedPlugin, ASWebAuthenticationPresentationContextProviding {
    public let identifier = "ChattoAuthorizationPlugin"
    public let jsName = "ChattoAuthorization"
    public let pluginMethods: [CAPPluginMethod] = [
        CAPPluginMethod(name: "authorize", returnType: CAPPluginReturnPromise)
    ]
    private var session: ASWebAuthenticationSession?
    private var pending: CAPPluginCall?
    private var timeout: DispatchWorkItem?
    private weak var anchor: ASPresentationAnchor?

    @objc public func authorize(_ call: CAPPluginCall) {
        DispatchQueue.main.async { [weak self] in
            self?.start(call)
        }
    }

    private func start(_ call: CAPPluginCall) {
        guard pending == nil else {
            call.reject("A sign-in attempt is already active.")
            return
        }
        guard let raw = call.getString("url"),
              let url = URL(string: raw), let host = url.host, !host.isEmpty,
              url.scheme == "https", url.user == nil, url.password == nil,
              let components = URLComponents(url: url, resolvingAgainstBaseURL: false),
              components.queryItems?.filter({ $0.name == "redirect_uri" }).map(\.value) == ["eu.chattocorp.chatto.mobile:/oauth/callback"],
              components.queryItems?.filter({ $0.name == "client_id" }).map(\.value) == ["eu.chattocorp.chatto.mobile"],
              let window = bridge?.viewController?.view.window else {
            call.reject("Sign-in requires an HTTPS server and the mobile callback.")
            return
        }
        pending = call
        anchor = window
        let attempt = ASWebAuthenticationSession(url: url, callbackURLScheme: "eu.chattocorp.chatto.mobile") { [weak self] callback, error in
            DispatchQueue.main.async {
                guard let self, self.pending === call else { return }
                self.timeout?.cancel()
                self.timeout = nil
                self.session = nil
                self.pending = nil
                self.anchor = nil
                guard error == nil, let callback,
                      callback.scheme == "eu.chattocorp.chatto.mobile", callback.host == nil,
                      callback.path == "/oauth/callback", callback.fragment == nil else {
                    // Do not expose provider URLs or native errors to logs.
                    call.reject("Sign-in was cancelled or could not be completed.")
                    return
                }
                call.resolve(["url": callback.absoluteString])
            }
        }
        attempt.presentationContextProvider = self
        session = attempt
        guard attempt.start() else {
            pending = nil
            session = nil
            anchor = nil
            call.reject("The sign-in session could not be started.")
            return
        }
        let expiry = DispatchWorkItem { [weak self] in
            guard let self, self.pending === call else { return }
            self.pending = nil
            self.session?.cancel()
            self.session = nil
            self.timeout = nil
            self.anchor = nil
            call.reject("The sign-in attempt timed out.")
        }
        timeout = expiry
        DispatchQueue.main.asyncAfter(deadline: .now() + 300, execute: expiry)
    }

    public func presentationAnchor(for session: ASWebAuthenticationSession) -> ASPresentationAnchor {
        anchor ?? ASPresentationAnchor()
    }

    deinit {
        timeout?.cancel()
        session?.cancel()
    }
}
