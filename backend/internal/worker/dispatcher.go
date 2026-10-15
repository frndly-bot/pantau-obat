package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/frndly-bot/pantau-obat/backend/internal/domain"
	"github.com/frndly-bot/pantau-obat/backend/internal/repo/postgres"
)

// Dispatcher mengambil item antrean yang jatuh tempo lalu membuat job
// pengiriman per kanal.
//
// Pemisahan scheduler dan dispatcher disengaja:
//
//   - Scheduler menjawab "kapan dosis jatuh tempo" dan berjalan tiap menit.
//   - Dispatcher menjawab "apa yang harus dikirim sekarang" dan berjalan
//     lebih sering (5 detik), karena ia yang menentukan ketepatan waktu
//     pengiriman.
//
// Keduanya berbagi tabel bump_queue. Bila digabung, satu loop harus
// melakukan dua hal yang frekuensinya berbeda, dan salah satunya akan
// dikorbankan.
type Dispatcher struct {
	db     *postgres.DB
	queue  *DBQueue
	logger *slog.Logger

	interval time.Duration

	// batchSize membatasi berapa item diambil per siklus, supaya satu
	// siklus tidak mengambil terlalu banyak dan menahan kunci terlalu lama.
	batchSize int

	// staleAfter menentukan kapan item yang diklaim dianggap terbengkalai.
	// Dilebihkan dari waktu pengiriman normal supaya item yang sedang
	// diproses tidak dilepas dan dikirim dua kali.
	staleAfter time.Duration

	// maxAttempts adalah batas percobaan sebelum item berhenti dilepas.
	// Tanpa batas ini, item yang selalu gagal akan berputar selamanya.
	maxAttempts int

	// senders adalah daftar kanal asli, disimpan supaya fanout bisa disusun
	// ULANG saat preferensi dipasang (WithPrefs dipanggil setelah
	// NewDispatcher, jadi preferensinya belum ada saat itu).
	senders []Sender

	// sender mengirim pesan ke kanal yang tersedia. Boleh nil; dalam hal itu
	// dispatcher mencatat job dengan status skipped, bukan diam-diam selesai.
	sender Sender

	// prefs menyediakan preferensi notifikasi user. Boleh nil, dan dalam hal
	// itu nilai bawaan dari domain dipakai.
	//
	// Interface, bukan tipe konkret, supaya paket worker tidak mengimpor
	// `service` — dan supaya loop ini bisa diuji tanpa database.
	prefs PrefsProvider
}

// PrefsProvider menyediakan preferensi notifikasi efektif seorang user.
//
// Dipisah dari Sender karena tanggung jawabnya berbeda: Sender menjawab
// "bagaimana mengirim", PrefsProvider menjawab "boleh kirim sekarang?".
type PrefsProvider interface {
	PrefsFor(ctx context.Context, userID string, channel domain.Channel) (domain.ChannelPrefs, error)
	ActivePushSubscriptions(ctx context.Context, ownerID string) ([]PushSubscription, error)
}

// Sender adalah antarmuka pengiriman notifikasi.
//
// Dibuat sebagai interface supaya dispatcher bisa diuji tanpa benar-benar
// mengirim push, dan supaya kanal baru cukup menambah implementasi tanpa
// mengubah loop-nya.
type Sender interface {
	// Send mengirim satu pesan. Mengembalikan id dari penyedia bila ada,
	// yang disimpan untuk melacak status kirim.
	Send(ctx context.Context, msg OutgoingMessage) (providerID string, err error)

	// Channel melaporkan kanal yang dilayani sender ini.
	Channel() domain.Channel

	// Available melaporkan apakah kanal siap dipakai. Kanal yang belum
	// dikonfigurasi (mis. email tanpa SMTP) tidak boleh membuat job gagal;
	// ia hanya dilewati.
	Available() bool
}

// OutgoingMessage adalah satu pesan yang siap dikirim.
type OutgoingMessage struct {
	OwnerID  string
	JobID    int64
	Kind     domain.JobKind
	Title    string
	Body     string
	Data     map[string]any
	DedupKey string
	// Email adalah alamat tujuan untuk kanal email.
	//
	// Ditambahkan bersama implementasi SMTP: sebelumnya field ini tidak ada
	// karena kanal email belum pernah dipakai, sehingga dispatcher tidak
	// punya cara memberi tahu pengirim ke alamat mana suratnya ditujukan.
	//
	// Dikosongkan untuk kanal lain. Pengirim email menolak pesan tanpa
	// alamat, bukan mengirimnya ke tempat yang tidak jelas.
	Email string
}

// WithPrefs memasang penyedia preferensi.
func (d *Dispatcher) WithPrefs(p PrefsProvider) *Dispatcher {
	d.prefs = p
	// Fanout disusun ULANG di sini, karena inilah saat preferensi tersedia.
	// Tanpa gerbang preferensi, kanal yang dimatikan user tetap dikirimi.
	d.sender = multiSender(d.senders, p, d.logger)
	return d
}

// NewDispatcher membuat dispatcher.
func NewDispatcher(db *postgres.DB, queue *DBQueue, logger *slog.Logger, senders ...Sender) *Dispatcher {
	return &Dispatcher{
		db:          db,
		queue:       queue,
		logger:      logger,
		interval:    5 * time.Second,
		batchSize:   50,
		staleAfter:  5 * time.Minute,
		maxAttempts: 5, // sesuai max_attempts di notification_jobs
		senders:     senders,
		sender:      multiSender(senders, nil, logger),
	}
}

// Run menjalankan loop sampai context dibatalkan.
func (d *Dispatcher) Run(ctx context.Context) error {
	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()

	// Siklus pertama dijalankan segera. Menunggu 5 detik pertama berarti
	// pengingat yang sudah jatuh tempo tertahan tanpa alasan.
	if err := d.RunOnce(ctx, time.Now()); err != nil {
		d.logger.ErrorContext(ctx, "dispatcher siklus awal gagal", slog.String("error", err.Error()))
	}

	for {
		select {
		case <-ctx.Done():
			return ErrWorkerStopped
		case <-ticker.C:
			if err := d.RunOnce(ctx, time.Now()); err != nil {
				d.logger.ErrorContext(ctx, "dispatcher gagal satu siklus",
					slog.String("error", err.Error()))
			}
		}
	}
}

// RunOnce menjalankan satu siklus pengiriman.
func (d *Dispatcher) RunOnce(ctx context.Context, now time.Time) error {
	// Lepas dulu item yang terbengkalai. Kalau proses mati di tengah
	// pengiriman, item itu tertinggal dalam keadaan claimed; tanpa dilepas,
	// pengingatnya hilang tanpa jejak.
	released, err := d.queue.ReleaseStaleBumps(ctx, d.queue.txOrPool(),
		now.Add(-d.staleAfter), d.maxAttempts)
	if err != nil {
		return err
	}
	if released > 0 {
		d.logger.WarnContext(ctx, "antrean terbengkalai dilepas",
			slog.Int64("jumlah", released))
	}

	items, err := d.queue.ClaimDueBumps(ctx, d.queue.txOrPool(), now, d.batchSize)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return nil
	}

	var done []int64
	for i := range items {
		it := &items[i]

		if err := d.dispatch(ctx, it); err != nil {
			// Item TIDAK ditandai selesai, sehingga akan dilepas lagi oleh
			// ReleaseStaleBumps dan dicoba ulang. Tapi hanya sampai
			// maxAttempts, supaya tidak berputar selamanya.
			d.logger.WarnContext(ctx, "pengiriman gagal",
				slog.Int64("bump_id", it.ID),
				slog.Int("attempts", it.Attempts),
				slog.String("error", err.Error()))
			continue
		}
		done = append(done, it.ID)
	}

	if len(done) > 0 {
		if err := d.queue.MarkBumpDone(ctx, d.queue.txOrPool(), done); err != nil {
			return err
		}
	}
	return nil
}

// dispatch mengirim satu item antrean ke semua kanal yang tersedia.
//
// Item dianggap selesai walau TIDAK ADA kanal yang tersedia. Alasannya
// penting: kalau kanal belum dikonfigurasi (mis. VAPID belum diisi),
// menandainya gagal akan membuat item berputar sampai maxAttempts lalu
// menghilang — dan saat kanal akhirnya dikonfigurasi, tumpukan item lama
// akan dikirim sekaligus.
func (d *Dispatcher) dispatch(ctx context.Context, it *BumpItem) error {
	// Job SELALU dicatat, walau tidak ada kanal yang tersedia.
	//
	// Sebelumnya ada jalan pintas "kalau sender nil, langsung selesai", dan
	// itu keliru: bump_queue ditandai selesai tanpa jejak apa pun di
	// notification_jobs maupun notification_deliveries. Akibatnya operator
	// yang bertanya "kenapa pengingat tidak sampai" tidak punya apa pun
	// untuk diperiksa — antreannya bersih, seolah semuanya terkirim.
	//
	// Pemisahan tanggung jawabnya jadi jelas: bump_queue menjawab "kapan",
	// notification_jobs menjawab "apa yang terjadi". Keduanya harus terisi,
	// termasuk ketika jawabannya "tidak dikirim karena kanal belum siap".
	msg, err := d.buildMessage(ctx, it)
	if err != nil {
		return err
	}
	if msg == nil {
		// Jenis antrean yang tidak dikenal. Dicatat sebagai selesai supaya
		// tidak berputar selamanya, dan tidak menggagalkan siklus.
		return nil
	}

	// Preferensi user diperiksa SEBELUM job dibuat, bukan sesudah.
	//
	// Alasannya: notifikasi yang ditahan jam sunyi bukan kegagalan, jadi
	// tidak boleh tercatat sebagai job gagal. Yang benar adalah menunda
	// bump-nya. Tapi bump_queue belum punya kolom waktu tunda terpisah;
	// karena itu penundaan dilakukan dengan MENGEMBALIKAN item ke antrean
	// pada waktu kirim yang baru.
	//
	// d.sender bisa nil (tidak ada kanal yang dikonfigurasi). channelOrDefault
	// dipakai di sini, BUKAN d.sender.Channel() langsung — pemanggilan
	// langsung dulu pernah menyebabkan nil pointer dereference dan panic di
	// tengah siklus dispatcher, karena pemeriksaan nil ada SETELAH bagian ini.
	if d.prefs != nil {
		channel := d.channelOrDefault()
		prefs, err := d.prefs.PrefsFor(ctx, it.OwnerID, channel)
		if err != nil {
			// Gagal membaca preferensi diperlakukan sebagai "kirim dengan
			// bawaan", bukan "tahan". Menahan notifikasi karena kesalahan
			// sementara database akan membuat pengingat obat hilang, dan
			// itu lebih merugikan daripada satu pengiriman yang salah waktu.
			d.logger.WarnContext(ctx, "gagal membaca preferensi notifikasi, memakai bawaan",
				slog.String("owner_id", it.OwnerID),
				slog.String("error", err.Error()))
			prefs = domain.DefaultPrefs(channel)
		}

		if !domain.ChannelEnabled(prefs) {
			// Kanal dimatikan user atau sedang ditahan. Item ditandai
			// selesai dengan alasan yang jelas, bukan dihapus diam-diam.
			return d.skipBump(ctx, it, "CHANNEL_DISABLED",
				"kanal notifikasi dimatikan atau sedang ditahan")
		}

		// Dihitung dalam zona waktu PENERIMA, bukan zona server: jam sunyi
		// adalah jam dinding di rumahnya. Lihat locPemilik.
		loc := d.locPemilik(ctx, it.OwnerID)
		lokal := it.ScheduledAt.In(loc)
		if sendAt := domain.EffectiveSendTime(lokal, prefs); sendAt.After(lokal) {
			// Jatuh di jam sunyi, atau dimajukan ke dalamnya. Ditunda.
			return d.deferBump(ctx, it, sendAt)
		}
	}

	// Job dicatat di notification_jobs, terpisah dari bump_queue. Job
	// menyimpan apa yang DIKIRIM beserta hasilnya; bump_queue hanya
	// menyimpan kapan sesuatu harus terjadi.
	jobID, err := d.db.CreateNotificationJob(ctx, d.queue.txOrPool(), postgres.JobInput{
		Kind:     msg.Kind,
		OwnerID:  msg.OwnerID,
		DedupKey: msg.DedupKey,
		Payload:  msg.Data,
		RunAt:    it.ScheduledAt,
	})
	if err != nil {
		return err
	}
	msg.JobID = jobID

	if d.sender == nil {
		// Tidak ada kanal yang siap. Job tetap tercatat, dengan hasil
		// "skipped" dan alasan yang jelas, sehingga operator bisa melihat
		// bahwa pengingatnya TIDAK dikirim dan kenapa.
		code := "NO_CHANNEL"
		reason := "tidak ada kanal notifikasi yang dikonfigurasi"
		if err := d.db.RecordDelivery(ctx, d.queue.txOrPool(), postgres.DeliveryInput{
			JobID:     jobID,
			Channel:   domain.ChannelInApp,
			Status:    domain.DeliverySkipped,
			ErrorCode: &code,
			ErrorMsg:  &reason,
		}); err != nil {
			d.logger.WarnContext(ctx, "gagal mencatat pengiriman yang dilewati",
				slog.Int64("job_id", jobID), slog.String("error", err.Error()))
		}
		d.logger.DebugContext(ctx, "pengingat dilewati: tidak ada kanal",
			slog.Int64("bump_id", it.ID), slog.Int64("job_id", jobID))
		return nil
	}

	providerID, err := d.sender.Send(ctx, *msg)
	if err != nil {
		// Kegagalan kirim DICATAT, tapi bukan sebagai error yang
		// menghentikan siklus: satu kanal yang bermasalah tidak boleh
		// menghambat pengiriman item berikutnya.
		d.recordDelivery(ctx, jobID, msg, "", err)
		return err
	}

	d.recordDelivery(ctx, jobID, msg, providerID, nil)
	return nil
}

// skipBump menandai item antrean selesai tanpa mengirim, dengan alasan.
//
// Job TETAP dicatat supaya ada jejak. Tanpa jejak, user yang bertanya
// "kenapa saya tidak dapat notifikasi" akan menemukan antrean yang bersih,
// dan tidak ada cara menjawabnya.
func (d *Dispatcher) skipBump(ctx context.Context, it *BumpItem, code, reason string) error {
	jobID, err := d.db.CreateNotificationJob(ctx, d.queue.txOrPool(), postgres.JobInput{
		Kind:     jobKindForBump(it.Kind),
		OwnerID:  it.OwnerID,
		DedupKey: "skipped:" + it.DedupKey,
		Payload: map[string]any{
			"skip_code":   code,
			"skip_reason": reason,
		},
		RunAt: it.ScheduledAt,
	})
	if err != nil {
		return err
	}

	if err := d.db.RecordDelivery(ctx, d.queue.txOrPool(), postgres.DeliveryInput{
		JobID:     jobID,
		Channel:   d.channelOrDefault(),
		Status:    domain.DeliverySkipped,
		ErrorCode: &code,
		ErrorMsg:  &reason,
	}); err != nil {
		d.logger.WarnContext(ctx, "gagal mencatat pelewatan notifikasi",
			slog.Int64("job_id", jobID), slog.String("error", err.Error()))
	}

	return d.queue.MarkBumpDone(ctx, d.queue.txOrPool(), []int64{it.ID})
}

// deferBump menunda pengiriman sampai waktu tertentu.
//
// Caranya: item lama ditandai selesai, lalu item BARU dibuat dengan
// scheduled_at yang bergeser. Tidak memakai kolom waktu tunda tersendiri di
// bump_queue, karena kolom itu belum ada dan menambahkannya berarti satu
// migrasi lagi untuk sesuatu yang bisa diselesaikan dengan cara ini.
//
// dedup_key diberi akhiran waktu tunda supaya uq_bump_dedup tidak menolak
// item baru selama item lama masih ada.
func (d *Dispatcher) deferBump(ctx context.Context, it *BumpItem, until time.Time) error {
	if err := d.queue.MarkBumpDone(ctx, d.queue.txOrPool(), []int64{it.ID}); err != nil {
		return err
	}

	// skip_tag menandai alasan penundaan, sehingga riwayatnya bisa ditelusuri
	// dan item yang tertunda berkali-kali bisa dikenali.
	tag := "quiet"
	key := it.DedupKey + "|deferred:" + until.UTC().Format(time.RFC3339)

	ok, err := d.queue.EnqueueBump(ctx, d.queue.txOrPool(), EnqueueInput{
		Kind:         it.Kind,
		OwnerID:      it.OwnerID,
		MedicationID: it.MedicationID,
		ScheduleID:   it.ScheduleID,
		ScheduledAt:  until,
		SlotDate:     it.SlotDate,
		DedupKey:     key,
		SkipTag:      &tag,
		// SubjectOwnerID HARUS ikut disalin.
		//
		// Ini sempat hilang dan akibatnya halus: bump yang ditunda karena
		// jam sunyi ditandai selesai lalu dibuat ulang TANPA pemilik
		// datanya. Untuk pengingat dosis itu tidak terasa (penerima dan
		// pemilik data orang yang sama), tapi pada eskalasi keduanya
		// berbeda — bump barunya kehilangan info pasiennya, pesannya tidak
		// bisa disusun, dan caregiver tidak pernah menerima pemberitahuan.
		// Gejalanya: bump selesai tanpa job, tanpa error di log.
		SubjectOwnerID: it.SubjectOwnerID,
	})
	if err != nil {
		return err
	}

	d.logger.DebugContext(ctx, "pengingat ditunda karena jam sunyi",
		slog.Int64("bump_id", it.ID),
		slog.String("sampai", until.Format(time.RFC3339)),
		slog.Bool("diantrekan", ok))
	return nil
}

// jobKindForBump memetakan jenis antrean ke jenis job.
func jobKindForBump(k domain.BumpKind) domain.JobKind {
	switch k {
	case domain.BumpDose:
		return domain.JobDoseReminder
	case domain.BumpRefill:
		return domain.JobRefillReminder
	case domain.BumpDigest:
		return domain.JobCareDigest
	}
	return domain.JobDoseReminder
}

// channelOrDefault melaporkan kanal yang dipakai sender.
func (d *Dispatcher) channelOrDefault() domain.Channel {
	if d.sender == nil {
		return domain.ChannelInApp
	}
	return d.sender.Channel()
}

// buildMessage menyusun isi pesan dari item antrean.
//
// NAMA OBAT diambil di sini karena memang dibutuhkan untuk isi notifikasi —
// pengingat yang berbunyi "waktunya minum obat" tanpa nama tidak berguna.
// Yang DILARANG adalah menaruhnya di log aplikasi dan di jejak audit.
func (d *Dispatcher) buildMessage(ctx context.Context, it *BumpItem) (*OutgoingMessage, error) {
	switch it.Kind {
	case domain.BumpDose:
		return d.buildDoseMessage(ctx, it)
	case domain.BumpRefill:
		return d.buildRefillMessage(ctx, it)
	case domain.BumpDigest:
		return d.buildDigestMessage(ctx, it)
	case domain.BumpEscalation:
		return d.buildEscalationMessage(ctx, it)
	}
	return nil, nil
}


// locPemilik memuat zona waktu penerima notifikasi.
//
// KENAPA INI PENTING
//
// Jam sunyi adalah konsep JAM DINDING pengguna: "jangan bangunkan saya
// antara 22:00 dan 06:00" berarti jam di rumahnya, bukan jam server.
// Sebelumnya pemeriksaan itu memakai jam server (UTC), dan akibatnya
// pengingat pagi untuk pengguna WIB — jam 05:00–13:00 WIB — SELALU
// dianggap jatuh di jam sunyi, lalu ditunda sampai 13:00 WIB. Dosis pagi,
// yang paling penting, datang terlambat 5–8 jam.
//
// Kegagalan memuat zona tidak menggagalkan pengiriman: dipakai zona bawaan.
// Pengingat obat yang datang tepat waktu lebih penting daripada penundaan
// yang presisi.
func (d *Dispatcher) locPemilik(ctx context.Context, ownerID string) *time.Location {
	tz := d.db.TimezoneOf(ctx, d.queue.txOrPool(), ownerID)
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return time.UTC
	}
	return loc
}

// emailPemilik mengambil alamat email pemilik untuk kanal email.
//
// Kegagalan TIDAK menggagalkan pembuatan pesan: kanal dalam aplikasi tetap
// harus jalan walau emailnya bermasalah. Yang terjadi adalah alamat kosong,
// dan pengirim email menolaknya dengan pesan yang jelas — bukan mengirim
// surat ke tempat yang tidak jelas.
func (d *Dispatcher) emailPemilik(ctx context.Context, ownerID string) string {
	u, err := d.db.UserByID(ctx, d.queue.txOrPool(), ownerID)
	if err != nil || u == nil {
		return ""
	}
	return u.Email
}

// buildDoseMessage menyusun pengingat dosis.
func (d *Dispatcher) buildDoseMessage(ctx context.Context, it *BumpItem) (*OutgoingMessage, error) {
	title := "Waktunya minum obat"
	body := "Buka aplikasi untuk konfirmasi"

	if it.MedicationID != nil {
		med, err := d.db.MedicationByIDAnyOwner(ctx, d.queue.txOrPool(), *it.MedicationID)
		if err == nil && med != nil {
			title = "Waktunya minum " + med.Name
			if med.StrengthNote != nil && *med.StrengthNote != "" {
				body = *med.StrengthNote
			} else {
				body = formatDose(med)
			}
		}
	}

	data := map[string]any{
		"type":          "dose_reminder",
		"medication_id": it.MedicationID,
		"schedule_id":   it.ScheduleID,
		"slot_date":     it.SlotDate.Format(time.DateOnly),
		"deep_link":     buildDeepLink(it),
		"actions":       "taken,snoozed,skipped",
	}

	return &OutgoingMessage{
		OwnerID:  it.OwnerID,
		Kind:     domain.JobDoseReminder,
		Title:    title,
		Body:     body,
		Data:     data,
		DedupKey: "dose:" + it.DedupKey,
		Email:    d.emailPemilik(ctx, it.OwnerID),
	}, nil
}

// buildRefillMessage menyusun pengingat restock.
func (d *Dispatcher) buildRefillMessage(ctx context.Context, it *BumpItem) (*OutgoingMessage, error) {
	title := "Obat hampir habis"
	body := "Waktunya membeli lagi"

	if it.MedicationID != nil {
		med, err := d.db.MedicationByIDAnyOwner(ctx, d.queue.txOrPool(), *it.MedicationID)
		if err == nil && med != nil {
			title = med.Name + " hampir habis"
		}
	}

	return &OutgoingMessage{
		OwnerID:  it.OwnerID,
		Kind:     domain.JobRefillReminder,
		Title:    title,
		Body:     body,
		Data:     map[string]any{"type": "refill_reminder", "medication_id": it.MedicationID},
		DedupKey: "refill:" + it.DedupKey,
		Email:    d.emailPemilik(ctx, it.OwnerID),
	}, nil
}

// buildEscalationMessage menyusun pemberitahuan untuk CAREGIVER bahwa
// pasiennya melewatkan beberapa hari dosis berturut-turut.
//
// PENERIMA BERBEDA DARI PEMILIK DATA. Ini satu-satunya jenis pesan yang
// seperti itu: `it.OwnerID` adalah caregiver yang menerima, sedangkan
// `it.SubjectOwnerID` adalah pasien yang datanya dibicarakan.
//
// Karena itu isinya dijaga ketat:
//
//   - TIDAK menyebut nama obat. Caregiver memang berhak tahu bahwa ada
//     masalah, tetapi rincian obat pasien adalah data kesehatan yang tidak
//     dibutuhkan untuk bertindak. Ia bisa membuka halaman pasien bila perlu,
//     dan pembukaan itu tercatat di audit.
//   - TIDAK menyebut dosis yang terlewat satu per satu. Yang berguna bagi
//     caregiver adalah polanya ("tiga hari berturut-turut"), bukan daftar
//     yang harus ia hitung sendiri.
func (d *Dispatcher) buildEscalationMessage(ctx context.Context, it *BumpItem) (*OutgoingMessage, error) {
	hari := 0
	if it.SubjectOwnerID != nil {
		if streak, err := d.db.MissedStreak(ctx, d.queue.txOrPool(),
			*it.SubjectOwnerID, it.SlotDate.Format(time.DateOnly)); err == nil {
			hari = streak
		}
	}

	nama := "Pasien yang Anda pantau"
	if it.SubjectOwnerID != nil {
		if u, err := d.db.UserByID(ctx, d.queue.txOrPool(), *it.SubjectOwnerID); err == nil && u != nil {
			nama = u.FullName
			if nama == "" {
				nama = "Pasien yang Anda pantau"
			}
		}
	}

	judul := "Ada dosis yang terlewat"
	isi := nama + " melewatkan dosis obat beberapa hari berturut-turut."
	if hari > 0 {
		isi = fmt.Sprintf("%s melewatkan dosis obat %d hari berturut-turut.", nama, hari)
	}
	isi += " Buka halaman pasien untuk melihat rinciannya."

	return &OutgoingMessage{
		OwnerID:  it.OwnerID, // penerima: caregiver
		Kind:     domain.JobMissedEscalation,
		Title:    judul,
		Body:     isi,
		Data: map[string]any{
			"type":            "missed_escalation",
			"subject_owner_id": it.SubjectOwnerID,
			"missed_days":      hari,
		},
		DedupKey: "escalation:" + it.DedupKey,
		// Alamat email PENERIMA (caregiver), bukan pemilik data. Salah di
		// sini berarti data kesehatan pasien dikirim ke alamat caregiver —
		// atau lebih buruk, pemberitahuan tentang pasien dikirim ke pasien.
		Email: d.emailPemilik(ctx, it.OwnerID),
	}, nil
}

// buildDigestMessage menyusun ringkasan harian.
func (d *Dispatcher) buildDigestMessage(ctx context.Context, it *BumpItem) (*OutgoingMessage, error) {
	return &OutgoingMessage{
		OwnerID:  it.OwnerID,
		Kind:     domain.JobCareDigest,
		Title:    "Ringkasan obat hari ini",
		Body:     "Lihat daftar obat yang diminum dan terlewat hari ini",
		Data:     map[string]any{"type": "daily_digest", "date": it.SlotDate.Format(time.DateOnly)},
		DedupKey: "digest:" + it.DedupKey,
		Email:    d.emailPemilik(ctx, it.OwnerID),
	}, nil
}

// recordDelivery mencatat hasil pengiriman per kanal.
//
// Kegagalan mencatat TIDAK dikembalikan sebagai error: yang penting
// pengirimannya sudah terjadi. Jejak yang hilang bisa diselidiki dari log.
func (d *Dispatcher) recordDelivery(ctx context.Context, jobID int64, msg *OutgoingMessage, providerID string, sendErr error) {
	status := domain.DeliverySent
	var code, errMsg *string
	if sendErr != nil {
		status = domain.DeliveryFailed
		c := "SEND_FAILED"
		m := sendErr.Error()
		code, errMsg = &c, &m
	}

	var pid *string
	if providerID != "" {
		pid = &providerID
	}

	if err := d.db.RecordDelivery(ctx, d.queue.txOrPool(), postgres.DeliveryInput{
		JobID:      jobID,
		Channel:    d.sender.Channel(),
		Status:     status,
		ProviderID: pid,
		ErrorCode:  code,
		ErrorMsg:   errMsg,
	}); err != nil {
		d.logger.WarnContext(ctx, "gagal mencatat hasil pengiriman",
			slog.Int64("job_id", jobID), slog.String("error", err.Error()))
	}

	if sendErr == nil {
		if err := d.db.MarkJobDone(ctx, d.queue.txOrPool(), jobID); err != nil {
			d.logger.WarnContext(ctx, "gagal menandai job selesai",
				slog.Int64("job_id", jobID), slog.String("error", err.Error()))
		}
	}
}

// formatDose menyusun teks dosis dari data obat.
func formatDose(med *domain.Medication) string {
	if med.StrengthNote != nil && *med.StrengthNote != "" {
		return *med.StrengthNote
	}
	return formatNumber(med.DoseValue) + " " + med.DoseUnitCode
}

// formatNumber menampilkan angka tanpa desimal bila bulat.
func formatNumber(v float64) string {
	if v == float64(int64(v)) {
		return itoaInt(int64(v))
	}
	return trimFloat(v)
}

func itoaInt(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// trimFloat menampilkan pecahan dengan tiga angka di belakang koma,
// lalu membuang nol yang tidak bermakna.
func trimFloat(v float64) string {
	whole := int64(v)
	frac := v - float64(whole)
	if frac < 0 {
		frac = -frac
	}
	digits := int64(frac*1000 + 0.5)
	if digits == 0 {
		return itoaInt(whole)
	}

	s := itoaInt(digits)
	for len(s) < 3 {
		s = "0" + s
	}
	for len(s) > 0 && s[len(s)-1] == '0' {
		s = s[:len(s)-1]
	}
	return itoaInt(whole) + "." + s
}

// buildDeepLink menyusun tautan langsung ke halaman dosis.
//
// Tanpa nama obat: tautannya memakai id, sehingga tautan yang tersalin atau
// tersimpan di riwayat browser tidak membocorkan data kesehatan.
func buildDeepLink(it *BumpItem) string {
	if it.MedicationID == nil {
		return "/hari-ini"
	}
	link := "/obat/" + *it.MedicationID
	link += "?date=" + it.SlotDate.Format(time.DateOnly)
	if it.ScheduleID != nil {
		link += "&sid=" + *it.ScheduleID
	}
	return link
}

// multiSender menggabungkan beberapa sender menjadi satu yang mencoba
// semuanya secara berurutan.
//
// Mengembalikan nil bila tidak ada satu pun yang tersedia, sehingga
// dispatcher tahu bahwa tidak ada kanal yang perlu dicoba.
// multiSender menyusun pengirim gabungan dari kanal yang benar-benar siap.
//
// `prefs` dipakai fanout untuk memeriksa tiap kanal PER PENERIMA. Di sini
// hanya kanal yang benar-benar tersedia yang dipilih: sender yang belum
// dikonfigurasi (mis. email tanpa SMTP) tidak boleh ikut, karena ia akan
// selalu gagal dan menutupi kanal yang sebenarnya bisa dipakai.
func multiSender(senders []Sender, prefs PrefsProvider, logger *slog.Logger) Sender {
	var available []Sender
	for _, s := range senders {
		if s != nil && s.Available() {
			available = append(available, s)
		}
	}
	if len(available) == 0 {
		return nil
	}
	return &fanoutSender{senders: available, prefs: prefs, logger: logger}
}

// fanoutSender mengirim ke SEMUA kanal yang aktif bagi penerimanya.
//
// Bukan "kirim ke kanal pertama yang berhasil". notification-v2 bagian 6
// menetapkan tiap kanal punya keputusan sendiri, dan bagian 5 mencatat
// alasannya: pasien dan caregiver memakai perangkat berbeda, jadi satu
// kanal tidak cukup. Mengirim hanya ke yang pertama membuat pengguna yang
// mengaktifkan email DAN in-app hanya menerima salah satunya.
type fanoutSender struct {
	senders []Sender
	prefs   PrefsProvider
	logger  *slog.Logger
}

// Channel melaporkan kanal yang mewakili pengirim ini.
//
// KANAL IN-APP DIDAHULUKAN, dan itu bukan soal urutan belaka.
//
// dispatcher memakai nilai ini untuk membaca preferensi sebagai gerbang
// sebelum mengirim. Sebelumnya fungsi ini mengembalikan kanal sender
// PERTAMA — dan begitu SMTP diaktifkan, sender pertama adalah email.
// dispatcher lalu menilai preferensi `email` untuk semua pesan; karena
// email mati secara bawaan, SELURUH pengingat dijatuhkan ke `skipped`
// CHANNEL_DISABLED — termasuk yang seharusnya masuk kotak masuk aplikasi.
//
// notification-v2 bagian 6 menetapkan `in_app` SELALU aktif. Mengembalikan
// in_app di sini membuat gerbang itu tidak pernah menolak semuanya; kanal
// lain tetap diperiksa sendiri di Send, per penerima.
func (f *fanoutSender) Channel() domain.Channel {
	for _, s := range f.senders {
		if s.Channel() == domain.ChannelInApp {
			return domain.ChannelInApp
		}
	}
	return f.senders[0].Channel()
}

// Available melaporkan apakah ada kanal yang bisa dipakai.
func (f *fanoutSender) Available() bool { return len(f.senders) > 0 }

// Send mengirim ke setiap kanal yang aktif bagi penerima pesan.
//
// Mengembalikan id penyedia dari kanal pertama yang berhasil, atau error
// terakhir bila semuanya gagal. Kegagalan satu kanal TIDAK menggagalkan
// pengiriman kanal lain: email yang bermasalah tidak boleh menahan
// notifikasi masuk ke kotak masuk aplikasi.
func (f *fanoutSender) Send(ctx context.Context, msg OutgoingMessage) (string, error) {
	var lastErr error
	var providerID string
	var terkirim int

	for _, s := range f.senders {
		ch := s.Channel()

		// Kanal in-app selalu lolos — ia tidak bisa mengganggu dan tidak
		// bergantung pada izin siapa pun (notification-v2 bagian 6).
		if ch != domain.ChannelInApp && f.prefs != nil {
			prefs, err := f.prefs.PrefsFor(ctx, msg.OwnerID, ch)
			if err != nil {
				// Gagal membaca preferensi diperlakukan sebagai "pakai
				// bawaan", bukan "jangan kirim": pengingat obat yang hilang
				// karena kesalahan database lebih merugikan.
				prefs = domain.DefaultPrefs(ch)
			}
			// Jam sunyi TIDAK diperiksa di sini: dispatcher sudah
			// memeriksanya memakai waktu jadwal bump sebelum memanggil
			// pengirim, dan di sini waktu itu tidak lagi tersedia.
			// Memeriksanya dengan waktu lain hanya akan menyesatkan.
			if !domain.ChannelEnabled(prefs) {
				continue
			}
		}

		pid, err := s.Send(ctx, msg)
		if err != nil {
			lastErr = err
			if f.logger != nil {
				f.logger.WarnContext(ctx, "pengiriman lewat satu kanal gagal",
					slog.String("channel", string(ch)),
					slog.String("owner_id", msg.OwnerID),
					slog.String("error", err.Error()))
			}
			continue
		}
		terkirim++
		if providerID == "" {
			providerID = pid
		}
	}

	if terkirim == 0 {
		if lastErr != nil {
			return "", lastErr
		}
		return "", errNoChannelActive
	}
	return providerID, nil
}

// errNoChannelActive: tidak ada kanal yang aktif bagi penerima ini.
//
// Bukan kegagalan teknis, tapi juga bukan keberhasilan. Dipisahkan supaya
// pemanggil bisa membedakannya dari error pengiriman sungguhan.
var errNoChannelActive = errors.New("tidak ada kanal notifikasi yang aktif untuk penerima ini")
