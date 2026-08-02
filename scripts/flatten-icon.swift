// Flattens a transparent-background logo onto a rounded black square for
// use as a macOS Dock icon. Bakes the corner rounding directly into the
// PNG's alpha channel — macOS does NOT auto-round a plain custom .icns for
// an unsigned/local-built app (confirmed: without this, the icon renders
// as a hard-cornered square next to every other app's properly rounded
// tile), so this has to be done ourselves.
//
// Two independent knobs, since the Dock tile has two separate proportions:
// squareScale controls how big the black rounded square itself is relative
// to the full 1024 canvas (Spotify-style icons leave a visible margin here,
// not full bleed), and glyphScale controls how big the logo mark is drawn
// within that square.
//
// Centers on the glyph's actual visible content (alpha bounding box), not
// the source image's own canvas — the source PNG has uneven internal
// padding (more space below the mark than above), so centering by canvas
// alone renders it looking visibly off-center.
import AppKit

let args = CommandLine.arguments
guard args.count == 5 else {
    print("usage: flatten-icon <src.png> <dst.png> <squareScale> <glyphScale>")
    exit(1)
}
let srcPath = args[1]
let dstPath = args[2]
let squareScale = Double(args[3]) ?? 1.0
let glyphScale = Double(args[4]) ?? 1.0

guard let src = NSImage(contentsOfFile: srcPath),
      let srcTiff = src.tiffRepresentation,
      let srcRep = NSBitmapImageRep(data: srcTiff) else {
    print("could not load \(srcPath)")
    exit(1)
}

let srcW = srcRep.pixelsWide, srcH = srcRep.pixelsHigh
var minX = srcW, maxX = 0, minY = srcH, maxY = 0
for y in 0..<srcH {
    for x in 0..<srcW {
        if let c = srcRep.colorAt(x: x, y: y), c.alphaComponent > 0.05 {
            if x < minX { minX = x }
            if x > maxX { maxX = x }
            if y < minY { minY = y }
            if y > maxY { maxY = y }
        }
    }
}
// Fraction (0-1) of the source canvas where the glyph's content is actually
// centered — not necessarily 0.5, since the source has uneven padding.
let contentCenterXFrac = (Double(minX) + Double(maxX)) / 2.0 / Double(srcW)
let contentCenterYFrac = (Double(minY) + Double(maxY)) / 2.0 / Double(srcH)

let canvas = NSSize(width: 1024, height: 1024)
let out = NSImage(size: canvas)
out.lockFocus()

let squareSize = NSSize(width: canvas.width * squareScale, height: canvas.height * squareScale)
let squareOrigin = NSPoint(x: (canvas.width - squareSize.width) / 2, y: (canvas.height - squareSize.height) / 2)
// Apple's icon template uses a corner radius of ~18.1% of the shape's width.
let cornerRadius = squareSize.width * 0.181
let clip = NSBezierPath(roundedRect: NSRect(origin: squareOrigin, size: squareSize), xRadius: cornerRadius, yRadius: cornerRadius)
clip.addClip()

NSColor.black.setFill()
NSRect(origin: squareOrigin, size: squareSize).fill()

let drawSize = NSSize(width: canvas.width * glyphScale, height: canvas.height * glyphScale)
// Solve for the draw origin such that the glyph's *content* center (not the
// raw image center) lands on the canvas center.
let canvasCenter = NSPoint(x: canvas.width / 2, y: canvas.height / 2)
let origin = NSPoint(
    x: canvasCenter.x - drawSize.width * contentCenterXFrac,
    y: canvasCenter.y - drawSize.height * (1.0 - contentCenterYFrac)
)
src.draw(in: NSRect(origin: origin, size: drawSize))
out.unlockFocus()

guard let tiff = out.tiffRepresentation, let rep = NSBitmapImageRep(data: tiff),
      let png = rep.representation(using: .png, properties: [:]) else {
    print("failed to encode png")
    exit(1)
}
try! png.write(to: URL(fileURLWithPath: dstPath))
print("wrote \(dstPath)")
