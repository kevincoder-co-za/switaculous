struct UTType: Hashable {
    var identifier: String
    var preferredFilenameExtension: String?

    init(exportedAs identifier: String, conformingTo parent: UTType? = nil) {
        self.identifier = identifier
        self.preferredFilenameExtension = nil
    }

    init(importedAs identifier: String, conformingTo parent: UTType? = nil) {
        self.identifier = identifier
        self.preferredFilenameExtension = nil
    }

    init(_ identifier: String, ext: String) {
        self.identifier = identifier
        self.preferredFilenameExtension = ext
    }

    init?(filenameExtension: String) {
        self.identifier = "dyn." + filenameExtension
        self.preferredFilenameExtension = filenameExtension
    }

    static let item = UTType("public.item", ext: "")
    static let content = UTType("public.content", ext: "")
    static let data = UTType("public.data", ext: "")
    static let folder = UTType("public.folder", ext: "")
    static let zip = UTType("public.zip-archive", ext: "zip")
    static let json = UTType("public.json", ext: "json")
    static let text = UTType("public.text", ext: "txt")
    static let plainText = UTType("public.plain-text", ext: "txt")
    static let utf8PlainText = UTType("public.utf8-plain-text", ext: "txt")
    static let commaSeparatedText = UTType("public.comma-separated-values-text", ext: "csv")
    static let pdf = UTType("com.adobe.pdf", ext: "pdf")
    static let image = UTType("public.image", ext: "")
    static let png = UTType("public.png", ext: "png")
    static let jpeg = UTType("public.jpeg", ext: "jpg")
    static let heic = UTType("public.heic", ext: "heic")
    static let movie = UTType("public.movie", ext: "mov")
    static let audio = UTType("public.audio", ext: "")
    static let mp3 = UTType("public.mp3", ext: "mp3")
    static let url = UTType("public.url", ext: "")
    static let xml = UTType("public.xml", ext: "xml")
    static let html = UTType("public.html", ext: "html")
    static let propertyList = UTType("com.apple.property-list", ext: "plist")
}
