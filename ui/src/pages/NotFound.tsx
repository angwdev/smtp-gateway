export function NotFound() {
  return (
    <div className="mx-auto max-w-prose p-6">
      <h1 className="text-2xl font-semibold">That page does not exist</h1>
      <p className="mt-2">
        <a href="/">Go to the overview</a>
      </p>
    </div>
  );
}
