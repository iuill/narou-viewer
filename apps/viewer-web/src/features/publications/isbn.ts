export function normalizeISBN13(value: string): string {
  const digits = Array.from(value.trim())
    .filter((character) => character >= "0" && character <= "9")
    .join("");
  if (digits.length !== 13) {
    return "";
  }
  if (!digits.startsWith("978") && !digits.startsWith("979")) {
    return "";
  }
  let sum = 0;
  for (let index = 0; index < 12; index += 1) {
    const digit = Number(digits[index]);
    sum += index % 2 === 0 ? digit : digit * 3;
  }
  const check = (10 - (sum % 10)) % 10;
  return check === Number(digits[12]) ? digits : "";
}
