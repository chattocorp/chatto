// Register one Carbon hotkey. The OS delivers both edges without input monitoring.
#import <AppKit/AppKit.h>
#include <Carbon/Carbon.h>
#include <dispatch/dispatch.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

static OSStatus edge(EventHandlerCallRef next, EventRef event, void *context) {
    puts(GetEventKind(event) == kEventHotKeyPressed ? "down" : "up");
    return noErr;
}

static int key_code(const char *name) {
    const char *names[] = {"Space", "Enter", "Left", "Right", "Up", "Down", "F1",  "F2",  "F3",
                           "F4",    "F5",    "F6",   "F7",    "F8", "F9",   "F10", "F11", "F12"};
    const int codes[] = {49,  36, 123, 124, 126, 125, 122, 120, 99,
                         118, 96, 97,  98,  100, 101, 109, 103, 111};
    for (size_t i = 0; i < sizeof(codes) / sizeof(*codes); i++)
        if (!strcmp(name, names[i]))
            return codes[i];
    if (strlen(name) != 1)
        return -1;

    // Resolve printable triggers on the current keyboard layout.
    TISInputSourceRef source = TISCopyCurrentASCIICapableKeyboardLayoutInputSource();
    if (!source)
        return -1;
    CFDataRef data = TISGetInputSourceProperty(source, kTISPropertyUnicodeKeyLayoutData);
    int match = -1;
    if (data) {
        const UCKeyboardLayout *layout = (const UCKeyboardLayout *)CFDataGetBytePtr(data);
        for (int code = 0; code < 128; code++) {
            UInt32 dead = 0;
            UniChar chars[4];
            UniCharCount length = 0;
            if (UCKeyTranslate(layout, code, kUCKeyActionDisplay, 0, LMGetKbdType(),
                               kUCKeyTranslateNoDeadKeysMask, &dead, 4, &length, chars) == noErr &&
                length == 1 &&
                (chars[0] == name[0] ||
                 (name[0] >= 'A' && name[0] <= 'Z' && chars[0] == name[0] + 32))) {
                match = code;
                break;
            }
        }
    }
    CFRelease(source);
    return match;
}

int main(int argc, char **argv) {
    if (argc != 3)
        return 1;
    setbuf(stdout, NULL);
    int mask = atoi(argv[1]), key = key_code(argv[2]);
    UInt32 mods = ((mask & 1) ? controlKey : 0) | ((mask & 2) ? cmdKey : 0) |
                  ((mask & 4) ? optionKey : 0) | ((mask & 8) ? shiftKey : 0);
    EventTypeSpec events[] = {{kEventClassKeyboard, kEventHotKeyPressed},
                              {kEventClassKeyboard, kEventHotKeyReleased}};
    NSApplication *app = NSApplication.sharedApplication;
    [app setActivationPolicy:NSApplicationActivationPolicyProhibited];
    EventHotKeyRef ref;
    EventHotKeyID id = {'CPTT', 1};
    if (key < 0 || InstallApplicationEventHandler(edge, 2, events, NULL, NULL) ||
        RegisterEventHotKey(key, mods, id, GetApplicationEventTarget(), kEventHotKeyExclusive, &ref))
        return 1;
    dispatch_source_t parent = dispatch_source_create(DISPATCH_SOURCE_TYPE_READ, STDIN_FILENO, 0,
                                                      dispatch_get_main_queue());
    dispatch_source_set_event_handler(parent, ^{
      exit(0);
    });
    dispatch_resume(parent);
    puts("ready");
    [app run];
    return 0;
}
