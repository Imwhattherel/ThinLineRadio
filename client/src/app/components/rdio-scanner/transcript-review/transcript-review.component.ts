import { Component, EventEmitter, Input, OnDestroy, OnInit, Output } from '@angular/core';
import { Subscription } from 'rxjs';
import { MatSnackBar } from '@angular/material/snack-bar';
import { RdioScannerTranscript } from '../rdio-scanner';
import { AlertsService } from '../alerts/alerts.service';
import { TranscriptReviewService } from './transcript-review.service';

@Component({
    selector: 'rdio-scanner-transcript-review',
    templateUrl: './transcript-review.component.html',
    styleUrls: ['./transcript-review.component.scss'],
})
export class RdioScannerTranscriptReviewComponent implements OnInit, OnDestroy {
    @Input() collectorConfigured = false;
    @Output() requestCollectorSetup = new EventEmitter<void>();

    transcripts: RdioScannerTranscript[] = [];
    loading = false;
    error = '';

    offset = 0;
    readonly limit = 50;
    hasMore = false;

    // Admin edit state
    editingCallId: number | null = null;
    editText = '';
    editSaving = false;
    editApproving = false;
    editAudioSrc = '';
    editAudioLoading = false;
    showTrainingTips = false;
    private editAudioObjectUrl: string | null = null;

    private pin = '';
    private loadSub?: Subscription;

    constructor(
        private alertsService: AlertsService,
        private reviewService: TranscriptReviewService,
        private snackBar: MatSnackBar,
    ) {}

    ngOnInit(): void {
        const raw = window?.localStorage?.getItem('rdio-scanner-pin');
        this.pin = raw ? window.atob(raw) : '';
        this.load();
    }

    ngOnDestroy(): void {
        this.loadSub?.unsubscribe();
        this.revokeEditAudio();
    }

    load(): void {
        this.loading = true;
        this.error = '';
        this.loadSub?.unsubscribe();
        this.loadSub = this.alertsService.getTranscripts(this.limit, this.offset, this.pin || undefined)
            .subscribe({
                next: (items) => {
                    this.transcripts = (items || []).map((t: any) => ({ ...t, transcript: t.transcript || '' }));
                    this.hasMore = this.transcripts.length >= this.limit;
                    this.loading = false;
                },
                error: (err) => {
                    this.error = err?.message || 'Failed to load transcripts';
                    this.transcripts = [];
                    this.loading = false;
                },
            });
    }

    prevPage(): void {
        this.offset = Math.max(0, this.offset - this.limit);
        this.load();
    }

    nextPage(): void {
        if (!this.hasMore) return;
        this.offset += this.limit;
        this.load();
    }

    formatTime(ts: number): string {
        if (!ts) return '';
        return new Date(ts).toLocaleString();
    }

    channelLabel(t: RdioScannerTranscript): string {
        const sys = t.systemLabel || `System ${t.systemId}`;
        const tg = t.talkgroupLabel || t.talkgroupName || `Talkgroup ${t.talkgroupId}`;
        return `${sys} / ${tg}`;
    }

    trackById(_i: number, t: RdioScannerTranscript): number | string {
        return t.callId ?? _i;
    }

    // ── Admin edit ────────────────────────────────────────────────────────────

    toggleEdit(t: RdioScannerTranscript): void {
        if (t.trainingReviewStatus === 'submitted') {
            return;
        }
        if (this.editingCallId === t.callId) {
            this.cancelEdit();
            return;
        }
        this.cancelEdit();
        this.editingCallId = t.callId ?? null;
        this.editText = t.reviewedTranscript?.trim() || t.transcript || '';
        if (t.callId != null) {
            this.loadEditAudio(t.callId);
        }
    }

    cancelEdit(): void {
        this.editingCallId = null;
        this.editText = '';
        this.revokeEditAudio();
    }

    saveDraft(): void {
        if (this.editingCallId == null || !this.editText.trim()) return;
        this.editSaving = true;
        this.reviewService.save(this.editingCallId, this.editText.trim())
            .then(() => {
                this.snackBar.open('Draft saved', '', { duration: 2500 });
            })
            .catch((e: any) => {
                this.snackBar.open(e?.error?.error || 'Save failed', '', { duration: 5000 });
            })
            .finally(() => { this.editSaving = false; });
    }

    approve(): void {
        if (this.editingCallId == null || !this.editText.trim()) return;
        if (!this.collectorConfigured) {
            this.snackBar.open('Connecting to transcript collector…', '', { duration: 3000 });
            this.requestCollectorSetup.emit();
            return;
        }
        this.editApproving = true;
        const approvedText = this.editText.trim().toUpperCase();
        this.reviewService.approve(this.editingCallId, approvedText)
            .then((res) => {
                this.snackBar.open(res.message || 'Approved & sent', '', { duration: 4000 });
                this.cancelEdit();
                this.load();
            })
            .catch((e: any) => {
                this.snackBar.open(e?.error?.error || 'Approve failed', '', { duration: 6000 });
            })
            .finally(() => { this.editApproving = false; });
    }

    private loadEditAudio(callId: number): void {
        this.revokeEditAudio();
        this.editAudioLoading = true;
        const url = this.reviewService.audioUrl(callId);
        const headers = this.reviewService.getAudioFetchHeaders();
        fetch(url, { headers })
            .then(r => { if (!r.ok) throw new Error(`HTTP ${r.status}`); return r.blob(); })
            .then(blob => {
                if (!blob.size) throw new Error('empty');
                this.editAudioObjectUrl = URL.createObjectURL(blob);
                this.editAudioSrc = this.editAudioObjectUrl;
                this.editAudioLoading = false;
            })
            .catch(() => {
                this.editAudioSrc = '';
                this.editAudioLoading = false;
            });
    }

    private revokeEditAudio(): void {
        if (this.editAudioObjectUrl) {
            URL.revokeObjectURL(this.editAudioObjectUrl);
            this.editAudioObjectUrl = null;
        }
        this.editAudioSrc = '';
        this.editAudioLoading = false;
    }
}
