import { parseIntent, matchWinWinOffers, showToast } from './api.js';
import { renderCheckoutButton } from './checkout.js';

export class NegotiatorApp {
  constructor() {
    this.ws = null;
    this.seq = 1;
    this.sku = 'CH-WINTER-PARKA-01';
    this.reconnectAttempts = 0;
    this.debounceTimer = null;
    this.countdownInterval = null;
    this.showingAllOffers = false;
    this.lastMatchResponse = null;

    this.initDOMElements();
    this.bindEvents();
    this.connectWebSocket();
  }

  initDOMElements() {
    this.budgetSlider = document.getElementById('budget-slider');
    this.budgetValue = document.getElementById('budget-val');
    this.flexSlider = document.getElementById('flex-slider');
    this.flexValue = document.getElementById('flex-val');
    this.bnplToggle = document.getElementById('bnpl-toggle');
 window.addEventListener('fairdeal-delivery-change',()=>this.scheduleConstraintsUpdate());
    this.bundleToggle = document.getElementById('bundle-toggle');

    this.aiPromptInput = document.getElementById('ai-prompt-input');
    this.btnAiParse = document.getElementById('btn-ai-parse');
    this.aiChip = document.getElementById('ai-chip');

    this.previewPrice = document.getElementById('preview-price');
    this.btnCommit = document.getElementById('btn-commit');

    this.offersSection = document.getElementById('winwin-offers-section');
    this.topOffersGrid = document.getElementById('top-offers-grid');
    this.allOffersContainer = document.getElementById('all-offers-container');
    this.allOffersGrid = document.getElementById('all-offers-grid');
    this.btnToggleAllOffers = document.getElementById('btn-toggle-all-offers');

    this.agreementCard = document.getElementById('agreement-card');
    this.selectedMerchantLabel = document.getElementById('selected-merchant-label');
    this.dealPriceBig = document.getElementById('deal-price-big');
    this.countdownBadge = document.getElementById('countdown-badge');
    this.paylaterCallout = document.getElementById('paylater-callout');
    this.selectedBuyerWin = document.getElementById('selected-buyer-win');
    this.selectedSellerWin = document.getElementById('selected-seller-win');
    this.concessionsList = document.getElementById('concessions-list');
    this.savingsTag = document.getElementById('savings-tag');

    this.terminalEl = document.getElementById('negotiation-terminal');
    this.terminalModelBadge = document.getElementById('terminal-model-badge');
    this.terminalStatus = document.getElementById('terminal-status');
    this.terminalDialogueBody = document.getElementById('terminal-dialogue-body');
    this.terminalFooter = document.getElementById('terminal-footer');

    this.agentAdviceBox = document.getElementById('agent-advice-box');
    this.agentAdviceText = document.getElementById('agent-advice-text');
    this.productFeaturesList = document.getElementById('product-features-list');
    this.prodDesc = document.getElementById('prod-desc');
    this.interestChipsContainer = document.getElementById('interest-chips');

    // Progressive 3-Stage Stepper elements
    this.stage1Container = document.getElementById('stage-1-container');
    this.gridStage2 = document.getElementById('grid-stage-2');
    this.gridStage3 = document.getElementById('grid-stage-3');

    this.stepBtn1 = document.getElementById('step-btn-1');
    this.stepBtn2 = document.getElementById('step-btn-2');
    this.stepBtn3 = document.getElementById('step-btn-3');
    this.step1Summary = document.getElementById('step-1-summary');
    this.step2Summary = document.getElementById('step-2-summary');
    this.step3Summary = document.getElementById('step-3-summary');

    this.btnSkipToLevers = document.getElementById('btn-skip-to-levers');
    this.btnBackTo1 = document.getElementById('btn-back-to-1');
    this.btnBackTo2 = document.getElementById('btn-back-to-2');
    this.btnToggleDialogue = document.getElementById('btn-toggle-dialogue');
    this.dialogueCollapsed = true;
  }

  setStage(stageNum) {
    if (!this.stage1Container || !this.gridStage2 || !this.gridStage3) return;

    const bVal = parseFloat(this.budgetSlider ? this.budgetSlider.value : 105).toFixed(0);
    const fPct = Math.round(parseFloat(this.flexSlider ? this.flexSlider.value : 0.85) * 100);

    if (this.stepBtn1) {
      this.stepBtn1.classList.toggle('active', stageNum === 1);
      this.stepBtn1.classList.toggle('completed', stageNum > 1);
    }
    if (this.stepBtn2) {
      this.stepBtn2.classList.toggle('active', stageNum === 2);
      this.stepBtn2.classList.toggle('completed', stageNum > 2);
    }
    if (this.stepBtn3) {
      this.stepBtn3.classList.toggle('active', stageNum === 3);
      this.stepBtn3.classList.remove('completed');
    }

    if (this.step2Summary && stageNum >= 2) {
      this.step2Summary.textContent = `Budget: $${bVal} • Flex: ${fPct}%`;
    }

    if (stageNum === 1) {
      this.stage1Container.style.display = 'block';
      this.gridStage2.style.display = 'none';
      this.gridStage3.style.display = 'none';
    } else if (stageNum === 2) {
      this.stage1Container.style.display = 'none';
      this.gridStage2.style.display = 'grid';
      this.gridStage3.style.display = 'none';
    } else if (stageNum === 3) {
      this.stage1Container.style.display = 'none';
      this.gridStage2.style.display = 'none';
      this.gridStage3.style.display = 'grid';
      if (this.step3Summary) {
        const count = this.lastMatchResponse && this.lastMatchResponse.top_offers
          ? this.lastMatchResponse.top_offers.length
          : 4;
        this.step3Summary.textContent = `${count} fictional merchants · compare packages`;
      }
    }

    window.scrollTo({ top: 0, behavior: 'smooth' });
  }

  bindEvents() {
    // Quick prompt inspiration pills
    document.querySelectorAll('.prompt-pill').forEach(pill => {
      pill.addEventListener('click', () => {
        if (this.aiPromptInput) {
          this.aiPromptInput.value = pill.dataset.prompt;
          this.btnAiParse.click();
        }
      });
    });
    this.budgetSlider.addEventListener('input', () => {
      this.budgetValue.textContent = `$${parseFloat(this.budgetSlider.value).toFixed(2)}`;
      this.scheduleConstraintsUpdate();
    });

    this.flexSlider.addEventListener('input', () => {
      const pct = Math.round(parseFloat(this.flexSlider.value) * 100);
      this.flexValue.textContent = `${pct}%`;
      this.scheduleConstraintsUpdate();
    });

    if (this.bnplToggle) this.bnplToggle.addEventListener('change', () => this.scheduleConstraintsUpdate());
    if (this.bundleToggle) this.bundleToggle.addEventListener('change', () => this.scheduleConstraintsUpdate());

    // Explicit accessory selection
    document.querySelectorAll('.interest-chip').forEach(chip => {
      chip.addEventListener('click', () => {
        chip.classList.toggle('active');
        this.scheduleConstraintsUpdate();
      });
    });

    if (this.btnToggleAllOffers) {
      this.btnToggleAllOffers.addEventListener('click', () => {
        this.showingAllOffers = !this.showingAllOffers;
        this.allOffersContainer.style.display = this.showingAllOffers ? 'flex' : 'none';
        const total = this.lastMatchResponse ? this.lastMatchResponse.all_offers.length : 5;
        this.btnToggleAllOffers.textContent = this.showingAllOffers
          ? 'Hide extra combinations ↑'
          : `Show all ${total} seller combinations ↓`;
      });
    }

    // Progressive 3-Stage Stepper navigation
    if (this.stepBtn1) this.stepBtn1.addEventListener('click', () => this.setStage(1));
    if (this.btnBackTo1) this.btnBackTo1.addEventListener('click', () => this.setStage(1));
    if (this.stepBtn2) this.stepBtn2.addEventListener('click', () => this.setStage(2));
    if (this.btnSkipToLevers) this.btnSkipToLevers.addEventListener('click', () => this.setStage(2));
    if (this.btnBackTo2) this.btnBackTo2.addEventListener('click', () => this.setStage(2));
    if (this.stepBtn3) {
      this.stepBtn3.addEventListener('click', () => {
        if (this.lastMatchResponse) {
          this.setStage(3);
        } else {
          this.runWinWinMatch();
        }
      });
    }

    if (this.btnToggleDialogue) {
      this.btnToggleDialogue.addEventListener('click', () => {
        this.dialogueCollapsed = !this.dialogueCollapsed;
        if (this.terminalDialogueBody) {
          this.terminalDialogueBody.style.display = this.dialogueCollapsed ? 'none' : 'flex';
        }
        if (this.terminalFooter) {
          this.terminalFooter.style.display = this.dialogueCollapsed ? 'none' : 'block';
        }
        this.btnToggleDialogue.textContent = this.dialogueCollapsed
          ? 'View Decision Trace ▼'
          : 'Hide Decision Trace ▲';
      });
    }

    this.btnAiParse.addEventListener('click', async () => {
      const text = this.aiPromptInput.value.trim();
      if (!text) return;
      this.btnAiParse.disabled = true;
      this.btnAiParse.textContent = 'Discovering Product & Levers...';

      try {
        const res = await parseIntent(text);

        // 1. Update Discovered Product info
        if (res.discovered_product) {
          const dp = res.discovered_product;
          if (this.prodDesc && dp.match_reason) {
            this.prodDesc.textContent = dp.match_reason;
          }
          if (this.productFeaturesList && dp.features && dp.features.length) {
            this.productFeaturesList.innerHTML = dp.features
              .map(f => `<span class="product-feature-tag">${escapeHtml(f)}</span>`)
              .join('');
          }
          if (this.step1Summary && dp.name) {
            this.step1Summary.textContent = `Matched: ${dp.name}`;
          }
        }

        // 2. Display Agent Strategy Advice
        if (this.agentAdviceBox && res.agent_advice) {
          this.agentAdviceBox.style.display = 'flex';
          if (this.agentAdviceText) {
            this.agentAdviceText.textContent = res.agent_advice;
          }
        }

        // 3. Dynamically Construct Fair Deal Bundle Chips based on the prompt
        if (this.interestChipsContainer && res.suggested_bundles && res.suggested_bundles.length) {
          this.interestChipsContainer.innerHTML = '';
          for (const b of res.suggested_bundles.filter(b => b.id === 'scarf')) {
            const span = document.createElement('span');
            span.className = `interest-chip ${b.active ? 'active' : ''}`;
            span.dataset.interest = b.id;
            span.title = b.reason || '';
            span.innerHTML = `<span class="ico"><svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="8" width="18" height="13" rx="2"/><path d="M12 8v13"/><path d="M3 12h18"/></svg></span> <span>${escapeHtml(b.name)}</span>`;
            span.addEventListener('click', () => {
              span.classList.toggle('active');
              this.scheduleConstraintsUpdate();
            });
            this.interestChipsContainer.appendChild(span);
          }

        }

        // 4. Update Sliders and Toggles
        if (res.constraints) {
          const c = res.constraints;
          const b = parseFloat(c.budget);
          if (!isNaN(b)) {
            this.budgetSlider.value = b;
            this.budgetValue.textContent = `$${b.toFixed(2)}`;
          }
          if (typeof c.shipping_flexibility === 'number') {
            this.flexSlider.value = c.shipping_flexibility;
            this.flexValue.textContent = `${Math.round(c.shipping_flexibility * 100)}%`;
          }
          if (this.bnplToggle && typeof c.prefers_pay_later === 'boolean') {
            this.bnplToggle.checked = c.prefers_pay_later;
          }
          if (this.bundleToggle && typeof c.willing_to_bundle === 'boolean') {
            this.bundleToggle.checked = c.willing_to_bundle;
          }

          this.aiChip.textContent = res.source === 'llm' ? 'AI mandate parser active' : 'Agent Rules Active';
          this.aiChip.style.display = 'inline-block';

          showToast('Product matched! Fine-tune your deal levers in Step 2.');
          this.scheduleConstraintsUpdate();
          this.setStage(2);
        }
      } catch (err) {
        showToast(err.message || 'Failed to parse text');
      } finally {
        this.btnAiParse.disabled = false;
        this.btnAiParse.innerHTML = '<span>Find Product &amp; Build Levers</span> <span class="ico"><svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round"><path d="M5 12h14m-7-7 7 7-7 7"/></svg></span>';
      }
    });

    this.btnCommit.addEventListener('click', () => this.runWinWinMatch());
  }

  getSelectedAccessories() {
    const active = [];
    document.querySelectorAll('.interest-chip.active').forEach(el => {
      if (el.dataset.interest) active.push(el.dataset.interest);
    });
    return active;
  }

  getConstraints() {
    return {
      budget: parseFloat(this.budgetSlider.value).toFixed(2),
 delivery_country: document.getElementById("delivery-country")?.value || "US",
      shipping_flexibility: parseFloat(this.flexSlider.value),
      prefers_pay_later: this.bnplToggle ? this.bnplToggle.checked : true,
      willing_to_bundle: this.getSelectedAccessories().includes('scarf') && (!this.bundleToggle || this.bundleToggle.checked)
    };
  }

  async runWinWinMatch() {
    this.agreementCard.style.display = 'none';
    this.lastMatchResponse = null;
    const submittedConstraints = JSON.stringify(this.getConstraints());
    this.btnCommit.disabled = true;
    this.btnCommit.textContent = 'Calculating a validated package...';

    try {
      const c = this.getConstraints();
      const matchRes = await matchWinWinOffers({
        sku: this.sku,
        budget: c.budget,
 delivery_country:c.delivery_country,
        shipping_flexibility: c.shipping_flexibility,
        prefers_pay_later: c.prefers_pay_later,
        willing_to_bundle: c.willing_to_bundle
      });

      if (submittedConstraints !== JSON.stringify(this.getConstraints())) {
        showToast('Your preferences changed. Calculate a new offer.');
        return;
      }
      this.lastMatchResponse = matchRes;
      if (matchRes.market_audit) {
        const badge = document.getElementById('kernel-audit-badge');
        const text = document.getElementById('kernel-audit-text');
        const lat = document.getElementById('kernel-audit-latency');
        if (badge && text) {
          badge.style.display = 'flex';
          text.textContent = matchRes.market_audit.status;
          if (lat) lat.textContent = matchRes.market_audit.verified ? 'Verified' : 'Not verified';
        }
      }
      if (matchRes.negotiation_log) {
        this.renderNegotiationDialogue(matchRes.negotiation_log);
      }
      this.renderOfferCards(matchRes);
      if (matchRes.top_offers && matchRes.top_offers.length > 0) {
        this.selectOffer(matchRes.top_offers[0]);
      }

      // Transition to Stage 3 (Side-by-side Offer Comparison + Sticky Checkout)
      this.setStage(3);
    } catch (err) {
      showToast(err.message || 'No Fairdeal deal possible at this budget. Try adjusting slightly.');
    } finally {
      this.btnCommit.disabled = false;
      this.btnCommit.innerHTML = '<span class="ico"><svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round"><line x1="18" y1="20" x2="18" y2="10"/><line x1="12" y1="20" x2="12" y2="4"/><line x1="6" y1="20" x2="6" y2="14"/></svg></span> <span>Calculate My Offer</span> <span class="ico"><svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round"><path d="M5 12h14m-7-7 7 7-7 7"/></svg></span>';
    }
  }

  renderNegotiationDialogue(log) {
    if (!this.terminalEl || !log || !log.dialogue) return;

    this.terminalEl.style.display = 'block';
    if (this.terminalModelBadge && log.model_used) {
      this.terminalModelBadge.textContent = log.model_used;
    }

    if (this.terminalStatus) {
      this.terminalStatus.textContent = 'Decision trace · validated agreement';
    }

    if (this.terminalDialogueBody) {
      this.terminalDialogueBody.innerHTML = '';
      for (const turn of log.dialogue) {
        const turnDiv = document.createElement('div');
        turnDiv.className = 'dialogue-turn';

        const isBuyer = turn.role === 'buyer';
        const bubble = document.createElement('div');
        bubble.className = `dialogue-bubble ${isBuyer ? 'bubble-buyer' : 'bubble-seller'}`;

        const speaker = turn.speaker || (isBuyer ? 'Buyer Agent' : 'Seller Agent');
        const intentHtml = turn.intent
          ? `<div class="bubble-intent">Intent: ${escapeHtml(turn.intent)}</div>`
          : '';

        bubble.innerHTML = `
          <div class="bubble-header">
            <span>${escapeHtml(speaker)}</span>
            <span style="font-size:0.68rem;opacity:0.75;font-family:var(--font-mono);">Turn ${turn.turn}</span>
          </div>
          <div>${escapeHtml(turn.message)}</div>
          ${intentHtml}
        `;

        turnDiv.appendChild(bubble);
        this.terminalDialogueBody.appendChild(turnDiv);
      }
    }

    if (this.terminalFooter) {
      const summaryReason = log.summary_reason
        ? `<div style="font-size:0.8rem;color:var(--heading);margin-bottom:0.35rem;">
            <strong>Solver decision:</strong> ${escapeHtml(log.summary_reason)}
          </div>`
        : '';

      this.terminalFooter.innerHTML = `
        ${summaryReason}
        <div class="consensus-strip">
          <span>Agreed Price: <strong>$${escapeHtml(log.agreed_price || '')}</strong></span>
          ${log.payment_plan ? `<span>&bull; ${escapeHtml(log.payment_plan)}</span>` : ''}
          ${log.included_gift ? `<span>&bull; ${escapeHtml(log.included_gift)}</span>` : ''}
          ${log.shipping_tier ? `<span>&bull; ${escapeHtml(log.shipping_tier)}</span>` : ''}
        </div>
      `;
    }

    // Default to collapsed so offers appear immediately at the top without scrolling
    this.dialogueCollapsed = true;
    if (this.terminalDialogueBody) this.terminalDialogueBody.style.display = 'none';
    if (this.terminalFooter) this.terminalFooter.style.display = 'none';
    if (this.btnToggleDialogue) this.btnToggleDialogue.textContent = 'View Decision Trace ▼';
  }

  renderOfferCards(matchRes) {
    this.offersSection.style.display = 'flex';
    if (this.btnToggleAllOffers) this.btnToggleAllOffers.style.display = 'none';
    this.allOffersContainer.style.display = 'none';
    this.topOffersGrid.innerHTML = '';
    this.allOffersGrid.innerHTML = '';

    const createCard = (offer, isTop) => {
      const div = document.createElement('div');
      div.className = 'offer-card';
      div.dataset.offerId = offer.offer_id;

      const giftLine = offer.concessions.bundle
        ? `<div style="font-size:0.78rem;color:var(--heading);font-weight:500;display:flex;align-items:center;gap:0.35rem;"><span class="ico"><svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="8" width="18" height="13" rx="2"/><path d="M12 8v13"/><path d="M3 12h18"/></svg></span><span>+ Free ${offer.concessions.bundle.name} ($${offer.concessions.bundle.retail_value} value)</span></div>`
        : `<div style="font-size:0.78rem;color:var(--text-muted);font-weight:300;">Single Item (No extra pack)</div>`;

      const payLine = offer.concessions.installments
        ? `Pay Later requested`
        : `Single payment`;

      const matchTag = offer.matched_interest
        ? `<span style="font-size:0.68rem;background:var(--bg-subtle);color:var(--heading);padding:0.15rem 0.5rem;border-radius:var(--radius-pill);font-weight:500;">Matched: "${offer.matched_interest}"</span>`
        : '';

      div.innerHTML = `
        <div>
          <div style="display:flex;justify-content:space-between;align-items:center;gap:0.35rem;margin-bottom:0.35rem;">
            <span class="offer-badge">${offer.badge}</span>
            <span style="font-size:0.72rem;font-weight:500;color:var(--text-muted);font-family:var(--font-mono);">Validated Match</span>
          </div>
          <div style="font-size:0.86rem;font-weight:500;color:var(--heading);">${offer.merchant_name}</div>
          <div style="display:flex;align-items:baseline;gap:0.5rem;margin:0.25rem 0;">
            <span style="font-size:1.4rem;font-weight:300;font-family:var(--font-mono);color:var(--heading);">$${offer.final_price}</span>
            <span style="font-size:0.74rem;color:var(--text-muted);font-weight:300;">${payLine}</span>
          </div>
          ${giftLine}
          <div style="font-size:0.74rem;color:var(--text-muted);margin-top:0.25rem;display:flex;align-items:center;gap:0.35rem;">
            <span class="ico"><svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round"><rect x="1" y="3" width="15" height="13"/><polygon points="16 8 20 8 23 11 23 16 16 16 16 8"/><circle cx="5.5" cy="18.5" r="2.5"/><circle cx="18.5" cy="18.5" r="2.5"/></svg></span>
            <span>${offer.concessions.shipping_tier === 'CONSOLIDATED_GROUND' ? 'Consolidated Ground' : 'Express Air'} (${offer.concessions.shipping_days}d) &bull; Free</span>
          </div>
        </div>
        <div style="display:flex;justify-content:space-between;align-items:center;border-top:1px solid var(--border);padding-top:0.45rem;margin-top:0.3rem;">
          <span style="font-size:0.74rem;font-weight:500;color:var(--heading);">$${offer.savings_vs_standard} Catalog savings</span>
          ${matchTag}
        </div>
      `;

      div.addEventListener('click', () => this.selectOffer(offer));
      return div;
    };

    for (const offer of matchRes.top_offers) {
      this.topOffersGrid.appendChild(createCard(offer, true));
    }
    for (const offer of matchRes.all_offers) {
      this.allOffersGrid.appendChild(createCard(offer, false));
    }
  }

  selectOffer(offer) {
 const standard=document.getElementById("selected-standard-total");if(standard)standard.textContent="$"+offer.standard_total;
    document.querySelectorAll('.offer-card').forEach(el => {
      el.classList.toggle('selected', el.dataset.offerId === offer.offer_id);
    });

    this.selectedMerchantLabel.textContent = `FAIRDEAL AGREEMENT READY • ${offer.merchant_name.toUpperCase()}`;
    this.dealPriceBig.textContent = `$${offer.final_price}`;
    if (this.selectedBuyerWin) this.selectedBuyerWin.textContent = offer.buyer_win_summary;
    if (this.selectedSellerWin) this.selectedSellerWin.textContent = offer.seller_win_summary;

    if (offer.concessions.installments) {
      this.paylaterCallout.style.display = 'block';
      this.paylaterCallout.textContent = 'PayPal Pay Later requested. Eligibility and terms are determined by PayPal at checkout.';
    } else {
      this.paylaterCallout.style.display = 'none';
    }

    this.concessionsList.innerHTML = '';
    const items = [
      'Winter Parka Performance 01',
      offer.concessions.shipping_tier === 'CONSOLIDATED_GROUND'
        ? `Consolidated Ground Shipping (${offer.concessions.shipping_days} days) — Free`
        : `Express Air Shipping (${offer.concessions.shipping_days} days) — Free`
    ];
    if (offer.concessions.bundle) {
      items.push(`${offer.concessions.bundle.name} ($${offer.concessions.bundle.retail_value} retail value) — Included Free in Pack`);
    }
    for (const itemText of items) {
      const li = document.createElement('li');
      li.textContent = itemText;
      this.concessionsList.appendChild(li);
    }

    this.savingsTag.innerHTML = `Catalog checkout savings: <strong>$${offer.savings_vs_standard}</strong> saved vs. standard checkout ($${offer.standard_total})`;

    this.startCountdown(offer.expires_at);
    this.agreementCard.style.display = 'flex';

    renderCheckoutButton(offer.agreement_id, offer.clearing_token, {
      final_price: offer.final_price,
      concessions: offer.concessions
    }, (agrId) => {
      if(this.bnplToggle)this.bnplToggle.checked=false;
      this.runWinWinMatch();
    });
  }

  scheduleConstraintsUpdate() {
    this.lastMatchResponse = null;
    this.agreementCard.style.display = 'none';
    if (this.countdownInterval) clearInterval(this.countdownInterval);
    clearTimeout(this.debounceTimer);
    this.debounceTimer = setTimeout(() => {
      this.send({
        event: 'CONSTRAINTS_UPDATE',
        seq: this.seq++,
        constraints: this.getConstraints()
      });
    }, 200);
  }

  connectWebSocket() {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${protocol}//${window.location.host}/v1/clearing/negotiate`;

    this.ws = new WebSocket(wsUrl);
    this.ws.onopen = () => {
      this.reconnectAttempts = 0;
      this.send({
        event: 'CLIENT_INITIATE',
        seq: this.seq++,
        sku: this.sku,
        constraints: this.getConstraints()
      });
    };

    this.ws.onmessage = (event) => {
      try {
        const msg = JSON.parse(event.data);
        this.handleMessage(msg);
      } catch (err) {
        console.error('WS parse error', err);
      }
    };
  }

  send(data) {
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify(data));
    }
  }

  handleMessage(msg) {
    if (msg.event === 'PREVIEW') {
      if (this.previewPrice) {
        if (msg.feasible) {
          this.previewPrice.textContent = `$${msg.estimated_price}`;
        } else {
          this.previewPrice.textContent = 'Min ~$84.00';
        }
      }
      // Never block the user from running the negotiation
      if (this.btnCommit) this.btnCommit.disabled = false;
    } else if (msg.event === 'NASH_AGREEMENT') {
      // Fallback for REQUOTE_WITHOUT_PAY_LATER
      this.selectOffer({
        offer_id: 'requote',
        agreement_id: msg.agreement_id,
        clearing_token: msg.clearing_token,
        merchant_name: 'Alpine Gear Co.',
        badge: 'Standard PayPal Requote',
        final_price: msg.agreement.final_price,
        standard_total: msg.agreement.standard_total,
        savings_vs_standard: msg.agreement.savings_vs_standard,
        buyer_win_summary: `Save $${msg.agreement.savings_vs_standard} with standard PayPal`,
        seller_win_summary: 'Protected margin with immediate PayPal settlement',
        concessions: msg.agreement.concessions,
        expires_at: msg.expires_at
      });
    }
  }

  startCountdown(expiresAtUnix) {
    if (this.countdownInterval) clearInterval(this.countdownInterval);
    const update = () => {
      const now = Math.floor(Date.now() / 1000);
      const remaining = expiresAtUnix - now;
      if (remaining <= 0) {
        clearInterval(this.countdownInterval);
        this.countdownBadge.textContent = 'Offer expired';
        this.countdownBadge.className = 'countdown-badge urgent';
        return;
      }
      const mins = Math.floor(remaining / 60);
      const secs = remaining % 60;
      this.countdownBadge.textContent = `Expires in ${String(mins).padStart(2, '0')}:${String(secs).padStart(2, '0')}`;
    };
    update();
    this.countdownInterval = setInterval(update, 1000);
  }
}

function escapeHtml(str) {
  if (typeof str !== 'string') return '';
  return str
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#039;');
}
