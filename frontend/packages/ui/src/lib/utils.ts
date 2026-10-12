/**
 * Utilitas kelas CSS.
 *
 * Versi ringan dari `cn()` (clsx + tailwind-merge) tanpa dependensi: cukup
 * untuk menyatukan kelas kondisional di komponen. Kelas Tailwind yang
 * bertabrakan tidak di-deduplikasi — komponen di paket ini ditulis agar tidak
 * mengirim kelas yang bertabrakan.
 */
export type ClassValue = string | number | null | undefined | false | ClassValue[];

export function cn(...values: ClassValue[]): string {
	const out: string[] = [];
	for (const value of values) {
		if (!value) continue;
		if (Array.isArray(value)) {
			const nested = cn(...value);
			if (nested) out.push(nested);
		} else {
			out.push(String(value));
		}
	}
	return out.join(' ');
}
