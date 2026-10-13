<script lang="ts">
	/**
	 * Dialog ubah data obat.
	 *
	 * KENAPA TERPISAH DARI WIZARD TAMBAH
	 *
	 * Wizard menangani 5 langkah termasuk pola frekuensi dan jam minum.
	 * Server TIDAK menerima perubahan frekuensi lewat `PATCH /medications/{id}`
	 * — yang bisa diubah adalah identitas obat (nama, dosis, rute, bentuk,
	 * catatan) beserta `is_active`. Mengubah jadwal adalah operasi tersendiri
	 * (kelola jam), dan mencampurnya di sini akan membuat pengguna mengira
	 * jadwalnya ikut berubah padahal tidak.
	 *
	 * Karena itu dialog ini sengaja hanya memuat field yang benar-benar
	 * tersimpan, dan menjelaskannya kepada pengguna.
	 */
	import { ApiError, type Medication } from '@pantau-obat/api-client';
	import { Button, Dialog, Input, Label } from '@pantau-obat/ui';
	import { api } from '$lib/api';
	import { i18n } from '$lib/i18n.svelte';

	type Props = {
		open?: boolean;
		med: Medication | null;
		onopenchange?: (open: boolean) => void;
		/** Dipanggil setelah perubahan tersimpan. */
		onsaved?: () => void;
	};

	let { open = $bindable(false), med, onopenchange, onsaved }: Props = $props();

	// Isian disalin dari obat saat dialog dibuka, bukan diikat langsung ke
	// `med`. Tanpa salinan, mengedit satu huruf akan langsung mengubah data
	// yang ditampilkan di halaman di belakangnya sebelum disimpan.
	let name = $state('');
	let doseValue = $state(1);
	let strengthNote = $state('');
	let notes = $state('');
	let notesReminder = $state('');
	let saving = $state(false);
	let error = $state<string | null>(null);

	/** Menyalin nilai obat ke isian setiap kali dialog dibuka. */
	$effect(() => {
		if (open && med) {
			name = med.name;
			doseValue = med.dose_value;
			strengthNote = med.strength_note ?? '';
			notes = med.notes ?? '';
			notesReminder = med.notes_reminder ?? '';
			error = null;
		}
	});

	function requestClose() {
		open = false;
		onopenchange?.(false);
	}

	async function save() {
		if (!med || saving) return;
		if (!name.trim()) {
			error = 'Nama obat belum diisi.';
			return;
		}

		saving = true;
		error = null;
		try {
			// Hanya field yang berubah yang dikirim. Mengirim semua field
			// membuat perubahan yang tidak disengaja ikut tersimpan, dan
			// menyulitkan penelusuran "siapa mengubah apa" di audit.
			const patch: Record<string, unknown> = {};
			if (name.trim() !== med.name) patch.name = name.trim();
			if (doseValue !== med.dose_value) patch.dose_value = doseValue;
			if ((strengthNote.trim() || null) !== med.strength_note)
				patch.strength_note = strengthNote.trim() || null;
			if ((notes.trim() || null) !== med.notes) patch.notes = notes.trim() || null;
			if ((notesReminder.trim() || null) !== med.notes_reminder)
				patch.notes_reminder = notesReminder.trim() || null;

			if (Object.keys(patch).length === 0) {
				requestClose();
				return;
			}

			await api.medications.update(med.id, patch);
			onsaved?.();
			requestClose();
		} catch (err) {
			error = err instanceof ApiError ? err.message : 'Gagal menyimpan perubahan.';
		} finally {
			saving = false;
		}
	}
</script>

<Dialog {open} title={i18n.t('common.edit')} onopenchange={(v) => !v && requestClose()}>
	<div class="space-y-4">
		<div class="space-y-2">
			<Label for="ed-name">{i18n.t('meds.medication_name')}</Label>
			<Input id="ed-name" bind:value={name} />
		</div>

		<div class="grid grid-cols-2 gap-3">
			<div class="space-y-2">
				<Label for="ed-dose">{i18n.t('meds.amount_per_dose')}</Label>
				<Input id="ed-dose" type="number" min="0.25" step="0.25" bind:value={doseValue} />
			</div>
			<div class="space-y-2">
				<Label>Satuan</Label>
				<p class="flex h-9 items-center rounded-md border border-border bg-muted/40 px-3 text-sm text-muted-foreground">
					{med?.dose_unit_code ?? '—'}
				</p>
			</div>
		</div>

		<div class="space-y-2">
			<Label for="ed-strength">{i18n.t('meds.strength_note')}</Label>
			<Input id="ed-strength" bind:value={strengthNote} />
		</div>

		<div class="space-y-2">
			<Label for="ed-notes">{i18n.t('meds.private_note')}</Label>
			<Input id="ed-notes" bind:value={notes} />
			<p class="text-xs text-muted-foreground">{i18n.t('meds.kept_yourself_never_visible_caregivers')}</p>
		</div>

		<div class="space-y-2">
			<Label for="ed-reminder">{i18n.t('meds.reminder_note')}</Label>
			<Input id="ed-reminder" bind:value={notesReminder} />
		</div>

		<!-- Batas yang disengaja, disebutkan supaya pengguna tidak mengira
		     jadwalnya ikut berubah. -->
		<p class="rounded-lg border border-border bg-muted/40 px-3.5 py-3 text-xs text-muted-foreground">
			Perubahan di sini tidak mengubah jadwal maupun jam minum. Untuk itu,
			gunakan tombol jam di panel Jadwal.
		</p>

		{#if error}
			<p class="rounded-md border border-destructive/30 bg-destructive/8 px-3 py-2 text-sm text-destructive" role="alert">
				{error}
			</p>
		{/if}
	</div>

	{#snippet footer()}
		<div class="flex justify-end gap-2">
			<Button variant="outline" onclick={requestClose}>{i18n.t('common.cancel')}</Button>
			<Button onclick={save} disabled={saving}>
				{saving ? `${i18n.t('common.save')}…` : i18n.t('common.save')}
			</Button>
		</div>
	{/snippet}
</Dialog>
