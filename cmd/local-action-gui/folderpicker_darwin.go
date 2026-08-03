//go:build darwin

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>
#include <string.h>

// Returns a heap-allocated (strdup'd) C string rather than relying on an
// autoreleased NSString buffer surviving across the cgo boundary — the
// caller owns it and must free it. Wrapped in @try/@catch so an ObjC-level
// exception gets logged (visible in the app's own log via NSLog going to
// stderr, and in Console.app) instead of silently taking down the whole
// process — a modal panel failing should be diagnosable, not a mystery.
const char* pickFolderNative(void) {
    @autoreleasepool {
        @try {
            NSOpenPanel *panel = [NSOpenPanel openPanel];
            [panel setCanChooseDirectories:YES];
            [panel setCanChooseFiles:NO];
            [panel setAllowsMultipleSelection:NO];
            [panel setCanCreateDirectories:YES];
            [panel setPrompt:@"Select"];
            NSInteger result = [panel runModal];
            if (result != NSModalResponseOK) {
                return strdup("");
            }
            NSURL *url = [[panel URLs] firstObject];
            if (url == nil) {
                return strdup("");
            }
            return strdup([url.path UTF8String]);
        } @catch (NSException *exception) {
            NSLog(@"pickFolderNative: %@: %@", exception.name, exception.reason);
            return strdup("");
        }
    }
}
*/
import "C"
import "unsafe"

// pickFolder opens a native "Choose a folder" dialog and returns the chosen
// absolute path, or "" if the user cancelled. Bound to window.pickRepoFolder
// so the frontend can call it directly — meaningful only in the DMG app: a
// regular browser tab has no way to expose a real filesystem path to JS at
// all (by design, for security), which is why the frontend only shows this
// affordance when that binding actually exists.
func pickFolder() string {
	cstr := C.pickFolderNative()
	defer C.free(unsafe.Pointer(cstr))
	return C.GoString(cstr)
}
