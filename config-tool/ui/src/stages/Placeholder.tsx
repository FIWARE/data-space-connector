/** Stands in for a stage that a later epic implements, so jumping there never fails. */
export function Placeholder({ label, comingIn }: { label: string; comingIn?: string }) {
  return (
    <section>
      <h1>{label} — not available yet</h1>
      {comingIn && <p className="lede">This stage comes with {comingIn}.</p>}
    </section>
  )
}
