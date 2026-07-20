/*
 * *****************************************************************************
 * Copyright (C) 2025 Thinline Dynamic Solutions
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>
 * ****************************************************************************
 */

import { HttpClient, HttpHeaders } from '@angular/common/http';
import { Injectable } from '@angular/core';
import { Observable, BehaviorSubject, of } from 'rxjs';
import { RdioScannerAlert, RdioScannerAlertPreference, RdioScannerIncidentUpdate, RdioScannerKeywordList } from '../rdio-scanner';

export interface RdioScannerSystemAlert {
    id: number;
    alertType: string;
    severity: string;
    title: string;
    message: string;
    data?: string;
    createdAt: number;
    createdBy?: number;
    dismissed?: boolean;
}

export interface RdioScannerSystemAlertsResponse {
    alerts: RdioScannerSystemAlert[];
    isSystemAdmin?: boolean;
    canViewSystemAlerts?: boolean;
}

export interface GlobalTrainingProgress {
    goalHours: number;
    submissions?: number;
    serverAccounts?: number;
    hoursDecimal?: number;
    percentOfGoal?: number;
    audioDurationMs?: number;
    audioDuration?: {
        hours?: number;
        minutes?: number;
        seconds?: number;
        formatted?: string;
    };
}

@Injectable()
export class AlertsService {
    private readonly apiUrl = '/api/alerts';
    private readonly preferencesUrl = '/api/alerts/preferences';
    private readonly keywordListsUrl = '/api/keyword-lists';
    private readonly transcriptsUrl = '/api/transcripts';

    // Optional base URL for cross-origin requests (e.g., when embedded in Central Management)
    private baseUrl: string | null = null;

    private readonly SESSION_KEY = 'tlr_alerts_cache';
    private readonly SESSION_MAX = 50;

    // Shared alerts cache - single source of truth
    private alertsCache: RdioScannerAlert[] = [];
    private lastFetchTime: number = 0;
    private isFetching: boolean = false;
    private alertsSubject: BehaviorSubject<RdioScannerAlert[]>;
    public alerts$: Observable<RdioScannerAlert[]>;

    constructor(private http: HttpClient) {
        // Restore from sessionStorage so the UI can paint cached data before the first HTTP response.
        const stored = this.readSessionCache();
        this.alertsCache = stored.alerts;
        this.lastFetchTime = stored.lastFetchTime;
        this.alertsSubject = new BehaviorSubject<RdioScannerAlert[]>([...this.alertsCache]);
        this.alerts$ = this.alertsSubject.asObservable();
    }

    private readSessionCache(): { alerts: RdioScannerAlert[]; lastFetchTime: number } {
        try {
            const raw = sessionStorage.getItem(this.SESSION_KEY);
            if (raw) {
                const parsed = JSON.parse(raw);
                return {
                    alerts: Array.isArray(parsed.alerts) ? parsed.alerts : [],
                    lastFetchTime: parsed.lastFetchTime || 0,
                };
            }
        } catch { /* ignore parse errors */ }
        return { alerts: [], lastFetchTime: 0 };
    }

    private writeSessionCache(): void {
        try {
            sessionStorage.setItem(this.SESSION_KEY, JSON.stringify({
                alerts: this.alertsCache.slice(0, this.SESSION_MAX),
                lastFetchTime: this.lastFetchTime,
            }));
        } catch { /* quota exceeded — silently skip */ }
    }

    /**
     * Set the base URL for API calls (used when embedded in Central Management)
     * @param url The base URL of the TLR server (e.g., 'https://scanner.example.com')
     */
    setBaseUrl(url: string | null): void {
        this.baseUrl = url ? url.replace(/\/$/, '') : null; // Remove trailing slash
    }

    /**
     * Get the full URL for an API endpoint
     */
    private getFullUrl(path: string): string {
        if (this.baseUrl) {
            return `${this.baseUrl}${path}`;
        }
        return path;
    }

    getAlerts(limit: number = 50, offset: number = 0, pin?: string): Observable<any[]> {
        let url = `${this.getFullUrl(this.apiUrl)}?limit=${limit}&offset=${offset}`;
        if (pin) {
            url += `&pin=${encodeURIComponent(pin)}`;
        }
        const headers = pin ? new HttpHeaders().set('Authorization', `Bearer ${pin}`) : undefined;
        return this.http.get<any[]>(url, { headers });
    }

    getSystemAlerts(limit: number = 50, pin?: string, includeDismissed: boolean = false): Observable<RdioScannerSystemAlertsResponse> {
        let url = `${this.getFullUrl('/api/system-alerts')}?limit=${limit}&includeDismissed=${includeDismissed}`;
        if (pin) {
            url += `&pin=${encodeURIComponent(pin)}`;
        }
        const headers = pin ? new HttpHeaders().set('Authorization', `Bearer ${pin}`) : undefined;
        return this.http.get<RdioScannerSystemAlertsResponse>(url, { headers });
    }

    /**
     * Fetch new alerts since last fetch time and append to cache
     * Returns the new alerts that were added
     */
    fetchNewAlerts(pin?: string, forceFullRefresh: boolean = false): Observable<RdioScannerAlert[]> {
        // Prevent concurrent fetches - return cached alerts if already fetching
        if (this.isFetching) {
            return new Observable<RdioScannerAlert[]>(observer => {
                // Return empty array if already fetching (will get updates via alerts$ subscription)
                observer.next([]);
                observer.complete();
            });
        }

        this.isFetching = true;

        let url = this.getFullUrl(this.apiUrl);
        const params: string[] = [];

        // If not forcing full refresh and we have a last fetch time, only get new alerts
        if (!forceFullRefresh && this.lastFetchTime > 0) {
            params.push(`since=${this.lastFetchTime}`);
        }

        if (pin) {
            params.push(`pin=${encodeURIComponent(pin)}`);
        }

        if (params.length > 0) {
            url += '?' + params.join('&');
        }

        const headers = pin ? new HttpHeaders().set('Authorization', `Bearer ${pin}`) : undefined;
        
        return new Observable<RdioScannerAlert[]>(observer => {
            this.http.get<any[]>(url, { headers }).subscribe({
                next: (newAlerts) => {
                    try {
                        const alertsToAdd: RdioScannerAlert[] = [];

                        if (forceFullRefresh) {
                            // Replace all alerts
                            this.alertsCache = (newAlerts || []).map((alert: any) => {
                                const typedAlert = alert as RdioScannerAlert;
                                typedAlert.transcriptSnippet = typedAlert.transcriptSnippet || typedAlert.transcript || '';
                                return typedAlert;
                            });
                            
                            // Set lastFetchTime to most recent alert's createdAt, or current time if no alerts
                            if (this.alertsCache.length > 0) {
                                // Alerts are returned in descending order (most recent first)
                                this.lastFetchTime = this.alertsCache[0].createdAt || Date.now();
                            } else {
                                this.lastFetchTime = Date.now();
                            }
                        } else {
                            // Convert and deduplicate
                            const existingIds = new Set(this.alertsCache.map(a => a.alertId));
                            
                            for (const alert of newAlerts || []) {
                                const typedAlert = alert as RdioScannerAlert;
                                typedAlert.transcriptSnippet = typedAlert.transcriptSnippet || typedAlert.transcript || '';
                                
                                // Only add if not already in cache
                                if (!existingIds.has(typedAlert.alertId)) {
                                    alertsToAdd.push(typedAlert);
                                }
                            }

                            // Append new alerts to the beginning (most recent first)
                            this.alertsCache = [...alertsToAdd, ...this.alertsCache];
                            
                            // Update lastFetchTime to most recent alert's createdAt
                            // If we got new alerts, use the first one (most recent), otherwise keep current time
                            if (alertsToAdd.length > 0) {
                                this.lastFetchTime = alertsToAdd[0].createdAt || this.lastFetchTime;
                            } else if (this.alertsCache.length > 0) {
                                // No new alerts, but we have cached ones - use the most recent cached alert
                                this.lastFetchTime = this.alertsCache[0].createdAt || this.lastFetchTime;
                            } else {
                                // No alerts at all - use current time
                                this.lastFetchTime = Date.now();
                            }
                        }

                        // Emit updated alerts and persist to sessionStorage
                        this.alertsSubject.next([...this.alertsCache]);
                        this.writeSessionCache();

                        this.isFetching = false;
                        observer.next(forceFullRefresh ? this.alertsCache : alertsToAdd);
                        observer.complete();
                    } catch (error) {
                        console.error('Error processing alerts:', error);
                        this.isFetching = false;
                        observer.error(error);
                    }
                },
                error: (error) => {
                    console.error('Error fetching new alerts:', error);
                    this.isFetching = false;
                    observer.error(error);
                }
            });
        });
    }

    /**
     * Get current alerts from cache
     */
    getCachedAlerts(): RdioScannerAlert[] {
        return [...this.alertsCache];
    }

    /**
     * Clear cache and reset
     */
    clearCache(): void {
        this.alertsCache = [];
        this.lastFetchTime = 0;
        this.alertsSubject.next([]);
        try { sessionStorage.removeItem(this.SESSION_KEY); } catch { /* ignore */ }
    }

    getTranscripts(limit: number = 50, offset: number = 0, pin?: string, systemId?: number, talkgroupId?: number, dateFrom?: number, dateTo?: number, search?: string): Observable<any[]> {
        let url = `${this.getFullUrl(this.transcriptsUrl)}?limit=${limit}&offset=${offset}`;
        if (pin) {
            url += `&pin=${encodeURIComponent(pin)}`;
        }
        if (systemId !== undefined && systemId !== null) {
            url += `&systemId=${systemId}`;
        }
        if (talkgroupId !== undefined && talkgroupId !== null) {
            url += `&talkgroupId=${talkgroupId}`;
        }
        if (dateFrom) {
            url += `&dateFrom=${dateFrom}`;
        }
        if (dateTo) {
            url += `&dateTo=${dateTo}`;
        }
        if (search && search.trim()) {
            url += `&search=${encodeURIComponent(search.trim())}`;
        }
        const headers = pin ? new HttpHeaders().set('Authorization', `Bearer ${pin}`) : undefined;
        return this.http.get<any[]>(url, { headers });
    }

    getTrainingProgress(pin?: string): Observable<GlobalTrainingProgress> {
        let url = `${this.getFullUrl('/api/transcripts/training-progress')}`;
        if (pin) {
            url += `?pin=${encodeURIComponent(pin)}`;
        }
        const headers = pin ? new HttpHeaders().set('Authorization', `Bearer ${pin}`) : undefined;
        return this.http.get<GlobalTrainingProgress>(url, { headers });
    }

    /**
     * Apply a pushed incident-mapping update to cached alerts for one call.
     */
    patchIncidentUpdate(update: RdioScannerIncidentUpdate): boolean {
        if (!update?.callId) {
            return false;
        }
        let changed = false;
        this.alertsCache = this.alertsCache.map((alert) => {
            if (alert.callId !== update.callId) {
                return alert;
            }
            changed = true;
            const next = { ...alert };
            if (update.incidentAddress !== undefined) {
                next.incidentAddress = update.incidentAddress;
            }
            if (update.incidentNature !== undefined) {
                next.incidentNature = update.incidentNature;
            }
            if (update.incidentLat !== undefined) {
                next.incidentLat = update.incidentLat;
            }
            if (update.incidentLon !== undefined) {
                next.incidentLon = update.incidentLon;
            }
            if (update.incidentGeocodeStatus !== undefined) {
                next.incidentGeocodeStatus = update.incidentGeocodeStatus;
            }
            return next;
        });
        if (changed) {
            this.alertsSubject.next([...this.alertsCache]);
            this.writeSessionCache();
        }
        return changed;
    }

    getPreferences(pin?: string): Observable<RdioScannerAlertPreference[]> {
        let url = this.getFullUrl(this.preferencesUrl);
        if (pin) {
            url += `?pin=${encodeURIComponent(pin)}`;
    }
        const headers = pin ? new HttpHeaders().set('Authorization', `Bearer ${pin}`) : undefined;
        return this.http.get<RdioScannerAlertPreference[]>(url, { headers });
    }

    updatePreferences(preferences: RdioScannerAlertPreference[], pin?: string): Observable<any> {
        const headers = pin ? new HttpHeaders().set('Authorization', `Bearer ${pin}`) : undefined;
        return this.http.put<any>(this.getFullUrl(this.preferencesUrl), preferences, { headers });
    }

    getKeywordLists(pin?: string): Observable<RdioScannerKeywordList[]> {
        let url = this.getFullUrl(this.keywordListsUrl);
        if (pin) {
            url += `?pin=${encodeURIComponent(pin)}`;
        }
        const headers = pin ? new HttpHeaders().set('Authorization', `Bearer ${pin}`) : undefined;
        return this.http.get<RdioScannerKeywordList[]>(url, { headers });
    }

    createKeywordList(list: Partial<RdioScannerKeywordList>, pin?: string): Observable<RdioScannerKeywordList> {
        const headers = pin ? new HttpHeaders().set('Authorization', `Bearer ${pin}`) : undefined;
        return this.http.post<RdioScannerKeywordList>(this.getFullUrl(this.keywordListsUrl), list, { headers });
    }

    updateKeywordList(listId: number, list: Partial<RdioScannerKeywordList>, pin?: string): Observable<any> {
        const headers = pin ? new HttpHeaders().set('Authorization', `Bearer ${pin}`) : undefined;
        return this.http.put<any>(`${this.getFullUrl(this.keywordListsUrl)}/${listId}`, list, { headers });
    }

    deleteKeywordList(listId: number, pin?: string): Observable<any> {
        const headers = pin ? new HttpHeaders().set('Authorization', `Bearer ${pin}`) : undefined;
        return this.http.delete<any>(`${this.getFullUrl(this.keywordListsUrl)}/${listId}`, { headers });
    }
}

