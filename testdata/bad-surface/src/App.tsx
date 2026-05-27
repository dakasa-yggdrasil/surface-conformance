// Deliberately violates every §3 invariant for fixture testing.

export function App() {
  async function loadFromStripe() {
    // §3.3 + §6.3 — direct provider call (hard error).
    const res = await fetch("https://api.stripe.com/v1/charges");
    return res.json();
  }

  async function loadCharges() {
    // §3.4 — /api/v1/* without instance scoping.
    const res = await fetch("/api/v1/charges");
    return res.json();
  }

  function persistOrder(orderId: string) {
    // §3.1 — writes business state to localStorage.
    localStorage.setItem(`order_total_${orderId}`, "1000.00");
  }

  function chargeUser(amount: number, balance: number) {
    // §3.6 — surface deciding business outcome locally.
    if (balance > amount) {
      return "charge";
    }
    return "decline";
  }

  // §3.5 — hardcoded AWS host at runtime.
  const ASSET_HOST = "https://my-bucket.s3.us-east-1.amazonaws.com";

  return (
    <div>
      <button onClick={loadFromStripe}>Stripe</button>
      <button onClick={loadCharges}>Charges</button>
      <button onClick={() => persistOrder("o1")}>Save</button>
      <button onClick={() => chargeUser(100, 200)}>Charge</button>
      <img src={`${ASSET_HOST}/logo.png`} />
    </div>
  );
}
