/*
 * *****************************************************************************
 * Copyright (C) 2019-2024 Chrystian Huot <chrystian@huot.qc.ca>
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

import { Injectable, OnDestroy } from '@angular/core';
import { MatDialog } from '@angular/material/dialog';
import { SwUpdate, VersionEvent } from '@angular/service-worker';
import { interval, Subscription } from 'rxjs';
import { AppUpdateComponent } from './update.component';

@Injectable()
export class AppUpdateService implements OnDestroy {
  private readonly subs = new Subscription();
  private prompting = false;
  private readonly onVisibility = (): void => {
    if (document.visibilityState === 'visible') {
      this.checkForUpdate();
    }
  };

  constructor(
    private matDialog: MatDialog,
    private ngSwUpdate: SwUpdate,
  ) {
    if (!ngSwUpdate.isEnabled) {
      return;
    }

    this.ngSwUpdate.versionUpdates.subscribe((event: VersionEvent) => {
      if (event.type === 'VERSION_READY') {
        this.prompt();
      }
    });

    // Do not wait for ApplicationRef.isStable — live feed websockets, audio, and
    // map polling keep the app perpetually "unstable", which delayed checks by minutes.
    if ('serviceWorker' in navigator) {
      navigator.serviceWorker.ready.then(() => this.checkForUpdate());
    }

    this.subs.add(interval(60_000).subscribe(() => this.checkForUpdate()));
    document.addEventListener('visibilitychange', this.onVisibility);
  }

  ngOnDestroy(): void {
    this.subs.unsubscribe();
    document.removeEventListener('visibilitychange', this.onVisibility);
  }

  private checkForUpdate(): void {
    if (!this.ngSwUpdate.isEnabled) {
      return;
    }
    this.ngSwUpdate.checkForUpdate().catch(() => undefined);
  }

  prompt(): void {
    if (this.prompting) {
      return;
    }
    this.prompting = true;
    this.matDialog.open(AppUpdateComponent, {
      panelClass: 'tlr-lcd-dialog',
      backdropClass: 'tlr-lcd-dialog-backdrop',
      autoFocus: 'dialog',
    }).afterClosed().subscribe((doUpdate) => {
      this.prompting = false;
      if (doUpdate) {
        if (this.ngSwUpdate.isEnabled) {
          this.ngSwUpdate.activateUpdate().then(() => document.location.reload());
        } else {
          document.location.reload();
        }
      }
    });
  }
}
