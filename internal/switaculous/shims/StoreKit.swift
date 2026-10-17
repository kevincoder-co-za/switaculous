// Preview stand-in: the store is always reachable and a purchase succeeds
// immediately without charging anyone. Purchased product IDs are remembered
// in UserDefaults so an unlock survives reloads, like a sandbox account would.

enum VerificationResult<SignedType> {
    case unverified(SignedType, Error)
    case verified(SignedType)

    var payloadValue: SignedType? {
        switch self {
        case .verified(let value): value
        case .unverified: nil
        }
    }
}

struct Transaction: Identifiable, Hashable {
    enum OwnershipType: Hashable {
        case purchased
        case familyShared
    }

    var id: UInt64
    var productID: String
    var purchaseDate: Date
    var revocationDate: Date?
    var expirationDate: Date?
    var ownershipType: OwnershipType = .purchased

    static let purchasesKey = "switaculous.storekit.purchasedProducts"

    static var purchasedProductIDs: [String] {
        get { UserDefaults.standard.stringArray(forKey: purchasesKey) ?? [] }
        set { UserDefaults.standard.set(newValue, forKey: purchasesKey) }
    }

    static func recordPurchase(of productID: String) -> Transaction {
        var owned = purchasedProductIDs
        if !owned.contains(productID) {
            owned.append(productID)
            purchasedProductIDs = owned
        }
        return Transaction(id: UInt64(Date().timeIntervalSince1970), productID: productID, purchaseDate: Date(), revocationDate: nil, expirationDate: nil)
    }

    static var currentEntitlements: [VerificationResult<Transaction>] {
        purchasedProductIDs.map { productID in
            .verified(Transaction(id: 1, productID: productID, purchaseDate: Date(), revocationDate: nil, expirationDate: nil))
        }
    }

    static var updates: [VerificationResult<Transaction>] { [] }
    static var all: [VerificationResult<Transaction>] { currentEntitlements }

    static func == (lhs: Transaction, rhs: Transaction) -> Bool { lhs.id == rhs.id }
    func hash(into hasher: inout Hasher) { hasher.combine(id) }

    func finish() async {}
}

struct Product: Identifiable, Hashable {
    enum ProductType: Hashable {
        case consumable
        case nonConsumable
        case nonRenewable
        case autoRenewable
    }

    enum PurchaseResult {
        case success(VerificationResult<Transaction>)
        case userCancelled
        case pending
    }

    enum PurchaseOption: Hashable {
        case quantity(Int)
        case appAccountToken(UUID)
    }

    var id: String
    var type: ProductType
    var displayName: String
    var description: String
    var price: Decimal
    var displayPrice: String
    var isFamilyShareable: Bool = false

    static func == (lhs: Product, rhs: Product) -> Bool { lhs.id == rhs.id }
    func hash(into hasher: inout Hasher) { hasher.combine(id) }

    static func products(for identifiers: [String]) async throws -> [Product] {
        identifiers.map { identifier in
            Product(id: identifier, type: .nonConsumable, displayName: identifier, description: "", price: 9.99, displayPrice: "$9.99")
        }
    }

    static func products(for identifiers: Set<String>) async throws -> [Product] {
        try await products(for: Array(identifiers))
    }

    func purchase(options: [PurchaseOption] = []) async throws -> PurchaseResult {
        .success(.verified(Transaction.recordPurchase(of: id)))
    }

    var latestTransaction: VerificationResult<Transaction>? { currentEntitlement }

    var currentEntitlement: VerificationResult<Transaction>? {
        Transaction.currentEntitlements.first { $0.payloadValue?.productID == id }
    }
}

enum AppStore {
    static var canMakePayments: Bool { true }

    static func sync() async throws {}

    static func requestReview(in scene: Any? = nil) {}

    static func showManageSubscriptions(in scene: Any? = nil) async throws {}
}

enum StoreKitError: Error {
    case unknown
    case userCancelled
    case networkError(Error)
    case systemError(Error)
    case notAvailableInStorefront
    case notEntitled
}
