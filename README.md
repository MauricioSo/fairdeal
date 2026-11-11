# Fairdeal

**Find a package that works for the buyer and the merchant, then check out with PayPal.**

Fairdeal is an AI-assisted commerce prototype built for the PayPal AI Hackathon.

## The idea

A buyer's budget may fall below a standard checkout total. A merchant may still be able to offer a feasible package when the buyer accepts flexible delivery or a stocked accessory. Fairdeal explores these trade-offs while enforcing the merchant's configured contribution margin.

## How it works

1. Describe a budget, delivery preference and willingness to receive an accessory.
2. Review the preferences proposed by the AI assistant.
3. A deterministic solver selects a feasible package using configured product, accessory, shipping and payment costs.
4. Review the exact offer and its decision trace before choosing whether to buy.
5. Approve a test payment through PayPal Sandbox.

The application does not automatically increase the buyer's maximum budget. Offers are persisted and signed, and payment orders use the stored agreement rather than a client-supplied price.

## Prototype scope

The current demonstration uses one seeded merchant, a winter parka and a cashmere scarf. Catalog prices, inventory and costs are demonstration data. Savings compare the demo catalog checkout, not independently verified market prices.

AI interprets buyer requests and proposes merchant policy drafts. The solver validates the economic constraints. The decision trace is not a live conversation between autonomous agents.

PayPal Sandbox transactions do not transfer live money. Pay Later availability and terms are determined by PayPal. No real-world conversion lift has been measured. Browsing history is not collected; submitted requests are processed by the application and its configured AI provider.

## Technology

Go, SQLite, embedded HTML/CSS/JavaScript, PayPal Orders v2 and the PayPal JavaScript SDK, AI-assisted intent parsing and policy compilation, and a Render deployment configuration.

## Repository contents

This public repository intentionally contains only this project overview. Application source code, credentials, environment files, transaction data and development history are not included. This overview alone is not the complete source-code submission required by the hackathon.

## Next steps

Deploy the demonstration, validate the complete Sandbox approval flow, integrate real inventory and fulfillment, implement source-backed price verification, and measure results in a pilot.
