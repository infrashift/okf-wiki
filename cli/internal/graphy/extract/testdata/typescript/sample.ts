import { helper } from "./util";

/** Shape of a point. */
export interface Point {
  x: number;
  y: number;
}

/** Named pair alias. */
export type Pair = [number, number];

/** Base drawable. */
export class Shape {
  area(): number {
    return 0;
  }
}

/** A circle. */
export class Circle extends Shape implements Point {
  x = 0;
  y = 0;

  area(): number {
    return Math.PI * helper(this.x);
  }
}

/** Compute total area. */
export const totalArea = (shapes: Shape[]): number => {
  return shapes.reduce((sum, s) => sum + s.area(), 0);
};

export function describe(c: Circle): string {
  return `circle ${c.area()}`;
}
