/**
 * Aksi Svelte: memunculkan elemen saat masuk viewport.
 *
 * Animasi minimal & ringan (poin keputusan 5): hanya `IntersectionObserver` +
 * transisi CSS yang sudah ada di `app.css`. Tidak ada pustaka animasi.
 * Otomatis tidak beranimasi bila pengguna memilih `prefers-reduced-motion`.
 */
export function reveal(node: HTMLElement, options: { delay?: number; once?: boolean } = {}) {
	const { delay = 0, once = true } = options;

	if (typeof IntersectionObserver === 'undefined') {
		node.dataset.reveal = 'visible';
		return {};
	}

	const observer = new IntersectionObserver(
		(entries) => {
			for (const entry of entries) {
				if (entry.isIntersecting) {
					if (delay > 0) {
						setTimeout(() => (node.dataset.reveal = 'visible'), delay);
					} else {
						node.dataset.reveal = 'visible';
					}
					if (once) observer.unobserve(node);
				} else if (!once) {
					node.dataset.reveal = '';
				}
			}
		},
		{ rootMargin: '0px 0px -10% 0px', threshold: 0.1 }
	);

	observer.observe(node);
	return {
		destroy() {
			observer.disconnect();
		},
	};
}
