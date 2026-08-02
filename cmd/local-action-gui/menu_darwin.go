//go:build darwin

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

// webview_go builds its window without ever constructing an NSApplication
// menu bar. AppKit routes Cmd+<key> shortcuts (copy, paste, cut, select
// all, undo/redo) through the menu bar's key equivalents, not directly to
// whatever view has focus — with no Edit menu claiming those key
// equivalents, the shortcuts have no destination and silently do nothing
// anywhere in the app, including plain text fields inside the WKWebView.
// This has to be built by hand since there's no nib/storyboard.
void installEditMenu(void) {
    @autoreleasepool {
        NSApplication *app = [NSApplication sharedApplication];

        NSMenu *menuBar = [app mainMenu];
        if (menuBar == nil) {
            menuBar = [[NSMenu alloc] init];
            [app setMainMenu:menuBar];
        }

        // CFBundleName ("local-action") reads better than the executable
        // name (NSProcessInfo.processName would give "local-action-gui").
        NSString *appName = [[NSBundle mainBundle] objectForInfoDictionaryKey:@"CFBundleName"];
        if (appName == nil) {
            appName = [[NSProcessInfo processInfo] processName];
        }

        NSMenuItem *appMenuItem = [[NSMenuItem alloc] init];
        [appMenuItem setTitle:appName];
        [menuBar addItem:appMenuItem];
        NSMenu *appMenu = [[NSMenu alloc] initWithTitle:appName];
        [appMenuItem setSubmenu:appMenu];
        [appMenu addItemWithTitle:[@"Quit " stringByAppendingString:appName]
                            action:@selector(terminate:)
                     keyEquivalent:@"q"];

        NSMenuItem *editMenuItem = [[NSMenuItem alloc] init];
        [menuBar addItem:editMenuItem];
        NSMenu *editMenu = [[NSMenu alloc] initWithTitle:@"Edit"];
        [editMenuItem setSubmenu:editMenu];

        [editMenu addItemWithTitle:@"Undo" action:@selector(undo:) keyEquivalent:@"z"];
        NSMenuItem *redo = [editMenu addItemWithTitle:@"Redo" action:@selector(redo:) keyEquivalent:@"z"];
        [redo setKeyEquivalentModifierMask:NSEventModifierFlagCommand | NSEventModifierFlagShift];
        [editMenu addItem:[NSMenuItem separatorItem]];
        [editMenu addItemWithTitle:@"Cut" action:@selector(cut:) keyEquivalent:@"x"];
        [editMenu addItemWithTitle:@"Copy" action:@selector(copy:) keyEquivalent:@"c"];
        [editMenu addItemWithTitle:@"Paste" action:@selector(paste:) keyEquivalent:@"v"];
        [editMenu addItemWithTitle:@"Select All" action:@selector(selectAll:) keyEquivalent:@"a"];
    }
}
*/
import "C"

func installEditMenu() {
	C.installEditMenu()
}
