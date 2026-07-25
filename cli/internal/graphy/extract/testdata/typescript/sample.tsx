/** Props for the counter view. */
export interface CounterProps {
  start: number;
  label: string;
}

/** Formats a count for display. */
export function formatCount(n: number): string {
  return `${n} item${n === 1 ? "" : "s"}`;
}

/** Counter renders a labelled tally. */
export function Counter({ start, label }: CounterProps) {
  const text = formatCount(start);
  return (
    <section className="counter">
      <h2>{label}</h2>
      <span>{text}</span>
    </section>
  );
}

/** Panel wraps a Counter with a heading. */
export const Panel = ({ title }: { title: string }) => {
  return (
    <div>
      <h1>{title}</h1>
      <Counter start={0} label="items" />
    </div>
  );
};

class Store {
  private count = 0;

  increment(): number {
    this.count += 1;
    return this.count;
  }
}
