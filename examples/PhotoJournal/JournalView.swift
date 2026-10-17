import SwiftUI
import SwiftData
import PhotosUI

struct JournalView: View {
    @Environment(\.modelContext) private var modelContext
    @Query(sort: \Entry.date, order: .reverse) private var entries: [Entry]
    @State private var pickedItem: PhotosPickerItem?
    @State private var showingCamera = false
    @State private var cameraPhoto: UIImage?

    var body: some View {
        NavigationStack {
            List {
                if entries.isEmpty {
                    ContentUnavailableView("No Entries", systemImage: "book.closed", description: Text("Add a photo from your library or take one with the camera."))
                }
                ForEach(entries) { entry in
                    NavigationLink(value: entry) {
                        EntryRow(entry: entry)
                    }
                }
                .onDelete(perform: deleteEntries)
            }
            .navigationTitle("Journal")
            .navigationDestination(for: Entry.self) { entry in
                EntryDetailView(entry: entry)
            }
            .toolbar {
                ToolbarItem(placement: .topBarLeading) {
                    Button {
                        showingCamera = true
                    } label: {
                        Label("Camera", systemImage: "camera")
                    }
                }
                ToolbarItem(placement: .topBarTrailing) {
                    PhotosPicker(selection: $pickedItem, matching: .images) {
                        Label("Add Photo", systemImage: "photo.badge.plus")
                    }
                }
            }
            .onChange(of: pickedItem) {
                Task {
                    if let data = try? await pickedItem?.loadTransferable(type: Data.self) {
                        modelContext.insert(Entry(title: "New memory", photo: data))
                    }
                    pickedItem = nil
                }
            }
            .onChange(of: cameraPhoto) {
                if let cameraPhoto, let data = cameraPhoto.jpegData(compressionQuality: 0.8) {
                    modelContext.insert(Entry(title: "Snapshot", photo: data))
                }
            }
            .fullScreenCover(isPresented: $showingCamera) {
                CameraPicker(image: $cameraPhoto)
                    .ignoresSafeArea()
            }
        }
    }

    private func deleteEntries(at offsets: IndexSet) {
        for index in offsets {
            modelContext.delete(entries[index])
        }
    }
}

struct EntryRow: View {
    let entry: Entry

    var body: some View {
        HStack(spacing: 12) {
            if let data = entry.photo, let image = UIImage(data: data) {
                Image(uiImage: image)
                    .resizable()
                    .scaledToFill()
                    .frame(width: 56, height: 56)
                    .clipShape(RoundedRectangle(cornerRadius: 10))
            } else {
                RoundedRectangle(cornerRadius: 10)
                    .fill(.gray.opacity(0.2))
                    .frame(width: 56, height: 56)
                    .overlay(Image(systemName: "photo").foregroundStyle(.secondary))
            }
            VStack(alignment: .leading, spacing: 4) {
                Text(entry.title)
                    .font(.headline)
                Text(entry.date, format: .dateTime.day().month().year())
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
            }
            Spacer()
            if entry.isFavorite {
                Image(systemName: "heart.fill")
                    .foregroundStyle(.red)
            }
        }
    }
}

struct EntryDetailView: View {
    @Bindable var entry: Entry

    var body: some View {
        Form {
            if let data = entry.photo, let image = UIImage(data: data) {
                Section {
                    Image(uiImage: image)
                        .resizable()
                        .scaledToFit()
                        .clipShape(RoundedRectangle(cornerRadius: 12))
                }
            }
            Section("Details") {
                TextField("Title", text: $entry.title)
                TextField("Notes", text: $entry.notes, axis: .vertical)
                    .lineLimit(3...6)
                DatePicker("Date", selection: $entry.date, displayedComponents: .date)
                Toggle("Favorite", isOn: $entry.isFavorite)
            }
        }
        .navigationTitle(entry.title)
        .navigationBarTitleDisplayMode(.inline)
    }
}

#Preview {
    JournalView()
        .modelContainer(for: Entry.self, inMemory: true)
}
