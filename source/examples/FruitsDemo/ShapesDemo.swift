import SwiftUI

struct ShapesDemo: View {
    @State private var progress = 0.65
    var body: some View {
        ScrollView {
            VStack(spacing: 24) {
                ZStack {
                    Circle()
                        .stroke(Color.gray.opacity(0.2), lineWidth: 14)
                    Circle()
                        .trim(from: 0, to: progress)
                        .stroke(Color.orange, style: StrokeStyle(lineWidth: 14, lineCap: .round))
                        .rotationEffect(.degrees(-90))
                    Text("\(Int(progress * 100))%")
                        .font(.title).bold()
                }
                .frame(width: 160, height: 160)

                HStack(spacing: 12) {
                    RoundedRectangle(cornerRadius: 16)
                        .fill(LinearGradient(colors: [.purple, .blue], startPoint: .topLeading, endPoint: .bottomTrailing))
                        .frame(width: 100, height: 100)
                    Capsule()
                        .fill(.mint)
                        .frame(width: 60, height: 100)
                    Circle()
                        .fill(.red.gradient)
                        .frame(width: 80, height: 80)
                }

                Text("Card title")
                    .font(.headline)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .padding()
                    .background(.blue.opacity(0.15), in: RoundedRectangle(cornerRadius: 12))
                    .padding(.horizontal)

                Button("Add 10%") {
                    withAnimation { progress = min(1, progress + 0.1) }
                }
                .buttonStyle(.bordered)
            }
            .padding(.top, 40)
        }
    }
}
