// Created by Jobs on 2026年11月18日，星期三.
// AppKit renders the actual captured Unicode cells; CoreImage then reads that font raster.
ObjC.import('AppKit');
ObjC.import('CoreImage');
function run(argv) {
    const text = ObjC.unwrap($.NSString.stringWithContentsOfFileEncodingError(argv[0], $.NSUTF8StringEncoding, null));
    // Keep blank quiet-zone rows; trim only the final newline from the file.
    const cells = text.replace(/\n$/, '').split('\n');
    const font = $.NSFont.fontWithNameSize('Menlo-Regular', 20);
    const attributes = $.NSMutableDictionary.alloc.init;
    attributes.setObjectForKey(font, $.NSFontAttributeName);
    const dark = argv[2] === 'dark';
    // ANSI30/47 overrides the surrounding light/dark terminal; plain mode keeps its real polarity.
    const ansi = argv[3] === 'ansi';
    const initialForeground = !dark ? $.NSColor.blackColor : $.NSColor.whiteColor;
    const initialBackground = !dark ? $.NSColor.whiteColor : $.NSColor.blackColor;
    const size = $('M').sizeWithAttributes(attributes);
    const cellWidth = Number(size.width);
    const lineHeight = cellWidth * 2;
    const width = Math.ceil(cells[0].replace(/\x1b\[[0-9;]*m/g, '').length * cellWidth);
    const height = Math.ceil(cells.length * lineHeight);
    const image = $.NSImage.alloc.initWithSize($.NSMakeSize(width, height));
    image.lockFocus;
    initialBackground.setFill;
    $.NSRectFill($.NSMakeRect(0, 0, width, height));
    for (let i = 0; i < cells.length; i++) {
        let foreground = initialForeground, background = initialBackground, column = 0;
        const parts = cells[i].match(/\x1b\[[0-9;]*m|[^\x1b]/g) || [];
        for (const part of parts) {
            if (part.startsWith('\x1b[')) {
                if (!ansi) continue;
                const numbers = part.slice(2, -1).split(';').map(Number);
                for (const number of numbers) {
                    if (number === 0) { foreground = initialForeground; background = initialBackground; }
                    if (number === 30) foreground = $.NSColor.blackColor;
                    if (number === 37) foreground = $.NSColor.whiteColor;
                    if (number === 40) background = $.NSColor.blackColor;
                    if (number === 47) background = $.NSColor.whiteColor;
                }
                continue;
            }
            const x = column * cellWidth, y = height - (i + 1) * lineHeight;
            background.setFill;
            $.NSRectFill($.NSMakeRect(x, y, cellWidth + .1, lineHeight + .1));
            attributes.setObjectForKey(foreground, $.NSForegroundColorAttributeName);
            $(part).drawAtPointWithAttributes($.NSMakePoint(x, y), attributes);
            column++;
        }
    }
    image.unlockFocus;
    const rep = $.NSBitmapImageRep.imageRepWithData(image.TIFFRepresentation);
    const data = rep.representationUsingTypeProperties($.NSPNGFileType, $.NSDictionary.dictionary);
    if (!data.writeToFileAtomically(argv[1], true)) throw Error('Cannot save terminal font raster');
    const detector = $.CIDetector.detectorOfTypeContextOptions($.CIDetectorTypeQRCode, $.CIContext.contextWithOptions($.NSDictionary.dictionary), $({CIDetectorAccuracy:'CIDetectorAccuracyHigh'}));
    const bitmap = $.CIImage.imageWithContentsOfURL($.NSURL.fileURLWithPath(argv[1]));
    const features = detector.featuresInImage(bitmap);
    const decoded = [];
    for (let i = 0; i < Number(features.count); i++) decoded.push(ObjC.unwrap(features.objectAtIndex(i).messageString));
    return JSON.stringify({background: dark ? 'dark' : 'light', mode: ansi ? 'ansi' : 'plain', width, height, decoded});
}
