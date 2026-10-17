import Foundation
import SwiftData

@Model
final class Entry {
    var title: String
    var notes: String
    var date: Date
    var isFavorite: Bool
    @Attribute(.externalStorage) var photo: Data?

    init(title: String, notes: String = "", date: Date = .now, isFavorite: Bool = false, photo: Data? = nil) {
        self.title = title
        self.notes = notes
        self.date = date
        self.isFavorite = isFavorite
        self.photo = photo
    }
}
