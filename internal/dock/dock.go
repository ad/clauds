package dock

import (
	"bytes"
	"encoding/binary"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ad/clauds/internal/paths"
)

const (
	execName = "clauds-open"
	iconName = "clauds.icns"
)

const plistTmpl = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleName</key>
	<string>%s</string>
	<key>CFBundleIdentifier</key>
	<string>com.github.ad.clauds.dock.%d</string>
	<key>CFBundleExecutable</key>
	<string>` + execName + `</string>
	<key>CFBundleIconFile</key>
	<string>` + iconName + `</string>
	<key>CFBundlePackageType</key>
	<string>APPL</string>
	<key>LSUIElement</key>
	<true/>
</dict>
</plist>
`

const badgeJS = `ObjC.import('AppKit');
function run(argv) {
  const [src, out, label, r, g, b] = argv;
  const S = 1024, full = $.NSMakeRect(0, 0, S, S);
  const img = $.NSImage.alloc.initWithContentsOfFile(src);
  if (img.isNil()) throw new Error('cannot load ' + src);
  const rep = $.NSBitmapImageRep.alloc.initWithBitmapDataPlanesPixelsWidePixelsHighBitsPerSampleSamplesPerPixelHasAlphaIsPlanarColorSpaceNameBytesPerRowBitsPerPixel(null, S, S, 8, 4, true, false, $.NSCalibratedRGBColorSpace, 0, 0);
  $.NSGraphicsContext.saveGraphicsState;
  $.NSGraphicsContext.setCurrentContext($.NSGraphicsContext.graphicsContextWithBitmapImageRep(rep));
  img.drawInRectFromRectOperationFraction(full, $.NSZeroRect, $.NSCompositingOperationSourceOver, 1);
  $.NSColor.colorWithCalibratedRedGreenBlueAlpha(Number(r), Number(g), Number(b), 1).setFill;
  $.NSRectFillUsingOperation(full, $.NSCompositingOperationHue);
  img.drawInRectFromRectOperationFraction(full, $.NSZeroRect, $.NSCompositingOperationDestinationIn, 1);
  const banner = $.NSMakeRect(40, 90, S - 80, 260);
  $.NSColor.colorWithCalibratedWhiteAlpha(0, 0.6).setFill;
  $.NSBezierPath.bezierPathWithRoundedRectXRadiusYRadius(banner, 70, 70).fill;
  let size = 200, attrs;
  for (;;) {
    attrs = $.NSMutableDictionary.alloc.init;
    attrs.setObjectForKey($.NSFont.boldSystemFontOfSize(size), 'NSFont');
    attrs.setObjectForKey($.NSColor.whiteColor, 'NSColor');
    if ($(label).sizeWithAttributes(attrs).width <= banner.size.width - 80 || size <= 60) break;
    size -= 8;
  }
  const t = $(label).sizeWithAttributes(attrs);
  $(label).drawAtPointWithAttributes($.NSMakePoint((S - t.width) / 2, banner.origin.y + (banner.size.height - t.height) / 2), attrs);
  $.NSGraphicsContext.restoreGraphicsState;
  const png = rep.representationUsingTypeProperties($.NSBitmapImageFileTypePNG, $());
  if (!png.writeToFileAtomically(out, true)) throw new Error('cannot write ' + out);
}`

var palette = [][3]float64{
	{0.20, 0.45, 0.95},
	{0.20, 0.70, 0.35},
	{0.60, 0.35, 0.90},
	{0.90, 0.30, 0.60},
	{0.10, 0.65, 0.70},
	{0.85, 0.65, 0.10},
	{0.85, 0.20, 0.20},
}

func BundlePath(dir, name string) string {
	return filepath.Join(dir, "Claude "+strings.ReplaceAll(name, "/", "-")+".app")
}

func Create(dir, name string, slot uint32, clauds, claudeApp string) (string, error) {
	icon, err := exec.Command("/usr/libexec/PlistBuddy", "-c", "Print :CFBundleIconFile",
		filepath.Join(claudeApp, "Contents", "Info.plist")).Output()
	if err != nil {
		return "", fmt.Errorf("reading Claude.app icon name: %w", err)
	}
	srcIcon := strings.TrimSpace(string(icon))
	if filepath.Ext(srcIcon) == "" {
		srcIcon += ".icns"
	}
	srcIcon = filepath.Join(claudeApp, "Contents", "Resources", srcIcon)

	app := BundlePath(dir, name)
	if paths.PathExists(app) && !ours(app) {
		return "", fmt.Errorf("%s exists and clauds did not make it", app)
	}

	tmp, err := os.MkdirTemp("", "clauds-icon")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	png := filepath.Join(tmp, "icon.png")
	c := palette[int(slot)%len(palette)]
	out, err := exec.Command("osascript", "-l", "JavaScript", "-e", badgeJS, srcIcon, png, name,
		fmt.Sprint(c[0]), fmt.Sprint(c[1]), fmt.Sprint(c[2])).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("drawing the icon: %w: %s", err, bytes.TrimSpace(out))
	}
	pngData, err := os.ReadFile(png)
	if err != nil {
		return "", err
	}

	contents := filepath.Join(app, "Contents")
	res := filepath.Join(contents, "Resources")
	for _, d := range []string{filepath.Join(contents, "MacOS"), res} {
		if err := os.MkdirAll(d, 0755); err != nil {
			return "", fmt.Errorf("creating %s: %w", app, err)
		}
	}

	var title bytes.Buffer
	xml.EscapeText(&title, []byte("Claude "+name))
	plist := fmt.Sprintf(plistTmpl, title.String(), slot)

	script := "#!/bin/sh\n"
	// Dock launches get launchd's environment, which lacks a custom XDG_CONFIG_HOME
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		script += "export XDG_CONFIG_HOME=" + shellQuote(xdg) + "\n"
	}
	script += "exec " + shellQuote(clauds) + " open " + shellQuote(name) + "\n"

	files := []struct {
		path string
		data []byte
		mode os.FileMode
	}{
		{filepath.Join(contents, "Info.plist"), []byte(plist), 0644},
		{filepath.Join(res, iconName), icns(pngData), 0644},
		{filepath.Join(contents, "MacOS", execName), []byte(script), 0755},
	}
	for _, f := range files {
		if err := os.WriteFile(f.path, f.data, f.mode); err != nil {
			return "", fmt.Errorf("writing %s: %w", f.path, err)
		}
	}
	return app, nil
}

func Remove(dir, name string) error {
	app := BundlePath(dir, name)
	if !ours(app) {
		return nil
	}
	if err := os.RemoveAll(app); err != nil {
		return fmt.Errorf("removing %s: %w", app, err)
	}
	return nil
}

func ours(app string) bool {
	return paths.PathExists(filepath.Join(app, "Contents", "MacOS", execName))
}

// A single 1024px PNG entry (ic10) is enough: macOS scales it for every size.
func icns(png []byte) []byte {
	var b bytes.Buffer
	b.WriteString("icns")
	binary.Write(&b, binary.BigEndian, uint32(16+len(png)))
	b.WriteString("ic10")
	binary.Write(&b, binary.BigEndian, uint32(8+len(png)))
	b.Write(png)
	return b.Bytes()
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
