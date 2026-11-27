// A call owns one Wayland portal session or X11 passive grab. On parent
// disconnect the process exits and the display server releases its resources.
#include <X11/XKBlib.h>
#include <X11/keysym.h>
#include <gio/gio.h>
#include <glib-unix.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

#define PORTAL "org.freedesktop.portal.Desktop"
#define PATH "/org/freedesktop/portal/desktop"
#define SHORTCUTS "org.freedesktop.portal.GlobalShortcuts"

static GDBusConnection *bus;
static char *request, *session, *shortcut_id, *trigger;
static Display *display;
static KeyCode trigger_code;

static gboolean parent_closed(gint fd, GIOCondition condition, gpointer data) { exit(0); }

static GVariant *options(const char *key, const char *value) {
    GVariantBuilder result;
    g_variant_builder_init(&result, G_VARIANT_TYPE_VARDICT);
    if (key)
        g_variant_builder_add(&result, "{sv}", key, g_variant_new_string(value));
    return g_variant_builder_end(&result);
}

static void portal_request(const char *method, GVariant *args) {
    GVariant *result =
        g_dbus_connection_call_sync(bus, PORTAL, PATH, SHORTCUTS, method, args,
                                    G_VARIANT_TYPE("(o)"), G_DBUS_CALL_FLAGS_NONE, -1, NULL, NULL);
    if (!result)
        exit(1);
    g_free(request);
    g_variant_get(result, "(o)", &request);
    g_variant_unref(result);
}

static void response(GDBusConnection *conn, const char *sender, const char *path, const char *iface,
                     const char *name, GVariant *args, gpointer data) {
    if (g_strcmp0(request, path))
        return;
    guint status;
    GVariant *results;
    g_variant_get(args, "(u@a{sv})", &status, &results);
    if (status)
        exit(1);
    if (!session) {
        // Portal implementations return the session path as either s or o.
        GVariant *handle = g_variant_lookup_value(results, "session_handle", NULL);
        if (!handle)
            exit(1);
        session = g_variant_dup_string(handle, NULL);
        g_variant_unref(handle);
        GVariantBuilder shortcuts, properties;
        g_variant_builder_init(&shortcuts, G_VARIANT_TYPE("a(sa{sv})"));
        g_variant_builder_init(&properties, G_VARIANT_TYPE_VARDICT);
        g_variant_builder_add(&properties, "{sv}", "description",
                              g_variant_new_string("Push to talk"));
        g_variant_builder_add(&properties, "{sv}", "preferred_trigger",
                              g_variant_new_string(trigger));
        g_variant_builder_add(&shortcuts, "(sa{sv})", shortcut_id, &properties);
        portal_request("BindShortcuts", g_variant_new("(oa(sa{sv})s@a{sv})", session, &shortcuts,
                                                      "", options("handle_token", "bind")));
    } else {
        GVariant *bindings =
            g_variant_lookup_value(results, "shortcuts", G_VARIANT_TYPE("a(sa{sv})"));
        if (!bindings)
            exit(1);
        GVariantIter iter;
        const char *id;
        gboolean found = FALSE;
        g_variant_iter_init(&iter, bindings);
        while (g_variant_iter_next(&iter, "(&s@a{sv})", &id, NULL))
            if (!strcmp(id, shortcut_id))
                found = TRUE;
        if (!found)
            exit(1);
        g_variant_unref(bindings);
        puts("ready");
    }
    g_variant_unref(results);
}

static void portal_event(GDBusConnection *conn, const char *sender, const char *path,
                         const char *iface, const char *name, GVariant *args, gpointer data) {
    if (!strcmp(name, "Closed") && !g_strcmp0(path, session))
        exit(1);
    if (strcmp(name, "Activated") && strcmp(name, "Deactivated"))
        return;
    const char *source, *id;
    g_variant_get_child(args, 0, "&o", &source);
    g_variant_get_child(args, 1, "&s", &id);
    if (!g_strcmp0(source, session) && !strcmp(id, shortcut_id))
        puts(!strcmp(name, "Activated") ? "down" : "up");
}

static void portal_lost(GDBusConnection *conn, const char *name, gpointer data) { exit(1); }

static void start_portal(int mask, const char *key, const char *app_id) {
    bus = g_bus_get_sync(G_BUS_TYPE_SESSION, NULL, NULL);
    if (!bus)
        exit(1);
    // The host registry is optional on older desktops.
    GVariant *registered = g_dbus_connection_call_sync(
        bus, PORTAL, PATH, "org.freedesktop.host.portal.Registry", "Register",
        g_variant_new("(s@a{sv})", app_id, options(NULL, NULL)), NULL, G_DBUS_CALL_FLAGS_NONE, -1,
        NULL, NULL);
    if (registered)
        g_variant_unref(registered);
    trigger = g_strdup_printf("%s%s%s%s%s", mask & 1 ? "CTRL+" : "", mask & 2 ? "LOGO+" : "",
                              mask & 4 ? "ALT+" : "", mask & 8 ? "SHIFT+" : "", key);
    // Portals persist assignments by action ID. A changed preference must
    // request its new trigger rather than reuse the old portal assignment.
    shortcut_id = g_strdup_printf("push-to-talk-%u-%s", mask, key);
    g_bus_watch_name_on_connection(bus, PORTAL, G_BUS_NAME_WATCHER_FLAGS_NONE, NULL, portal_lost,
                                   NULL, NULL);
    g_dbus_connection_signal_subscribe(bus, PORTAL, "org.freedesktop.portal.Request", "Response",
                                       NULL, NULL, 0, response, NULL, NULL);
    g_dbus_connection_signal_subscribe(bus, PORTAL, SHORTCUTS, NULL, PATH, NULL, 0, portal_event,
                                       NULL, NULL);
    g_dbus_connection_signal_subscribe(bus, PORTAL, "org.freedesktop.portal.Session", "Closed",
                                       NULL, NULL, 0, portal_event, NULL, NULL);
    portal_request("CreateSession",
                   g_variant_new("(@a{sv})", options("session_handle_token", "chatto_ptt")));
}

static int grab_error(Display *d, XErrorEvent *event) { exit(1); }
static gboolean x_events(gint fd, GIOCondition condition, gpointer data) {
    if (condition & (G_IO_ERR | G_IO_HUP))
        exit(1);
    while (XPending(display)) {
        XEvent event;
        XNextEvent(display, &event);
        if ((event.type == KeyPress || event.type == KeyRelease) &&
            event.xkey.keycode == trigger_code)
            puts(event.type == KeyPress ? "down" : "up");
    }
    return G_SOURCE_CONTINUE;
}

static void start_x11(int mask, const char *key) {
    display = XOpenDisplay(NULL);
    if (!display)
        exit(1);
    trigger_code = XKeysymToKeycode(display, XStringToKeysym(key));
    Bool repeat;
    if (!trigger_code || !XkbSetDetectableAutoRepeat(display, True, &repeat) || !repeat)
        exit(1);
    unsigned mods = ((mask & 1) ? ControlMask : 0) | ((mask & 2) ? Mod4Mask : 0) |
                    ((mask & 4) ? Mod1Mask : 0) | ((mask & 8) ? ShiftMask : 0);
    unsigned locks = LockMask | XkbKeysymToModifiers(display, XK_Num_Lock) |
                     XkbKeysymToModifiers(display, XK_Scroll_Lock);
    XSetErrorHandler(grab_error);
    for (unsigned extra = 0; extra < 256; extra++)
        if (!(extra & ~locks))
            XGrabKey(display, trigger_code, mods | extra, DefaultRootWindow(display), False,
                     GrabModeAsync, GrabModeAsync);
    XSync(display, False);
    puts("ready");
    x_events(ConnectionNumber(display), G_IO_IN, NULL);
    g_unix_fd_add(ConnectionNumber(display), G_IO_IN | G_IO_ERR | G_IO_HUP, x_events, NULL);
}

int main(int argc, char **argv) {
    if (argc != 4)
        return 1;
    setbuf(stdout, NULL);
    int mask = atoi(argv[1]);
    const char *key = argv[2];
    char letter[2] = {0};
    if (strlen(key) == 1 && g_ascii_isalnum(*key)) {
        letter[0] = g_ascii_tolower(*key);
        key = letter;
    } else if (!strcmp(key, "/"))
        key = "slash";
    else if (!strcmp(key, ","))
        key = "comma";
    else if (!strcmp(key, "Enter"))
        key = "Return";
    else if (!strcmp(key, "Space"))
        key = "space";
    g_unix_fd_add(STDIN_FILENO, G_IO_IN | G_IO_HUP | G_IO_ERR, parent_closed, NULL);
    if (g_getenv("WAYLAND_DISPLAY") || !g_strcmp0(g_getenv("XDG_SESSION_TYPE"), "wayland"))
        start_portal(mask, key, argv[3]);
    else
        start_x11(mask, key);
    g_main_loop_run(g_main_loop_new(NULL, FALSE));
}
