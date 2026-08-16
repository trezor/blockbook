export class SkipTest extends Error {}

export function errorMessage(error: unknown) {
  return error instanceof Error ? error.message : String(error);
}
