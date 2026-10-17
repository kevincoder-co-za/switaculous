import SwiftUI

struct Recipe: Identifiable, Hashable {
    let id = UUID()
    var name: String
    var minutes: Int
    var emoji: String
    var isFavorite: Bool
    var steps: [String]
}

final class RecipeBook: ObservableObject {
    @Published var recipes: [Recipe] = [
        Recipe(name: "Bobotie", minutes: 60, emoji: "🍛", isFavorite: true,
               steps: ["Fry the onions", "Add mince and spices", "Pour over the egg topping", "Bake for 30 minutes"]),
        Recipe(name: "Pancakes", minutes: 20, emoji: "🥞", isFavorite: false,
               steps: ["Mix the batter", "Heat the pan", "Cook until golden"]),
        Recipe(name: "Greek Salad", minutes: 10, emoji: "🥗", isFavorite: true,
               steps: ["Chop the vegetables", "Add feta and olives", "Drizzle with olive oil"]),
    ]
}

struct RecipeRow: View {
    var recipe: Recipe
    var body: some View {
        HStack(spacing: 12) {
            Text(recipe.emoji)
                .font(.largeTitle)
                .frame(width: 56, height: 56)
                .background(Color.orange.opacity(0.15), in: RoundedRectangle(cornerRadius: 12))
            VStack(alignment: .leading, spacing: 2) {
                Text(recipe.name).font(.headline)
                Label("\(recipe.minutes) min", systemImage: "clock")
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
            }
            Spacer()
            if recipe.isFavorite {
                Image(systemName: "heart.fill").foregroundStyle(.pink)
            }
        }
    }
}

struct FavoriteButton: View {
    @Binding var isSet: Bool
    var body: some View {
        Button {
            isSet.toggle()
        } label: {
            Label("Favorite", systemImage: isSet ? "heart.fill" : "heart")
                .labelStyle(.iconOnly)
                .foregroundStyle(isSet ? .pink : .gray)
                .font(.title2)
        }
    }
}

struct RecipeDetail: View {
    @EnvironmentObject var book: RecipeBook
    var recipe: Recipe

    var index: Int {
        book.recipes.firstIndex(where: { $0.id == recipe.id })!
    }

    var body: some View {
        ScrollView {
            ZStack {
                LinearGradient(colors: [.orange, .pink], startPoint: .topLeading, endPoint: .bottomTrailing)
                    .frame(height: 220)
                Text(recipe.emoji)
                    .font(.system(size: 100))
                    .shadow(radius: 10)
            }
            VStack(alignment: .leading, spacing: 12) {
                HStack {
                    Text(recipe.name).font(.title.bold())
                    Spacer()
                    FavoriteButton(isSet: $book.recipes[index].isFavorite)
                }
                Label("Ready in \(recipe.minutes) minutes", systemImage: "timer")
                    .foregroundStyle(.secondary)
                Divider()
                Text("Steps").font(.title2.bold())
                ForEach(Array(recipe.steps.enumerated()), id: \.offset) { number, step in
                    HStack(alignment: .top, spacing: 12) {
                        Text("\(number + 1)")
                            .font(.headline)
                            .foregroundStyle(.white)
                            .frame(width: 28, height: 28)
                            .background(Circle().fill(.orange))
                        Text(step)
                    }
                }
            }
            .padding()
        }
        .navigationTitle(recipe.name)
        .navigationBarTitleDisplayMode(.inline)
    }
}

struct RecipeList: View {
    @EnvironmentObject var book: RecipeBook
    @State private var favoritesOnly = false

    var shown: [Recipe] {
        book.recipes.filter { !favoritesOnly || $0.isFavorite }
    }

    var body: some View {
        NavigationStack {
            List {
                Toggle("Favorites only", isOn: $favoritesOnly)
                ForEach(shown) { recipe in
                    NavigationLink {
                        RecipeDetail(recipe: recipe)
                    } label: {
                        RecipeRow(recipe: recipe)
                    }
                }
            }
            .navigationTitle("Recipes")
        }
    }
}

#Preview {
    RecipeList()
        .environmentObject(RecipeBook())
}
