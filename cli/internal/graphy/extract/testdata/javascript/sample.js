import { helper } from "./util.js";

/** Base drawable. */
export class Shape {
  area() {
    return 0;
  }
}

/** A square. */
export class Square extends Shape {
  constructor(side) {
    super();
    this.side = side;
  }

  area() {
    return helper(this.side) * this.side;
  }
}

/** Sum all areas. */
export const totalArea = (shapes) => shapes.reduce((sum, s) => sum + s.area(), 0);

export function describe(sq) {
  return `square ${sq.area()}`;
}
