import React, { useState } from 'react';
import Link from '@docusaurus/Link';
import useDocusaurusContext from '@docusaurus/useDocusaurusContext';
import Layout from '@theme/Layout';
import Heading from '@theme/Heading';
import styles from './index.module.css';

function HomepageHeader() {
  const { siteConfig } = useDocusaurusContext();
  return (
    <header className={styles.heroSection}>
      <div className={styles.heroGridBg} />
      <div className={styles.glowOrb} />
      <div className={styles.glowOrbLeft} />
      <div className={styles.heroContent}>
        <Heading as="h1" className={styles.heroTitle}>
          {siteConfig.title}
        </Heading>
        <p className={styles.heroTagline}>{siteConfig.tagline}</p>
        <div className={styles.heroButtons}>
          <Link className={styles.primaryButton} to="/docs">
            Get Started &rarr;
          </Link>
          <Link className={styles.secondaryButton} to="/docs/sdk">
            Explore Go SDK
          </Link>
        </div>
      </div>
    </header>
  );
}

const CODE_TEMPLATES = {
  curl: `// Submitting an egress request via cURL REST API
curl -s -H "Authorization: Bearer sk_example_req_returned_once" \\
  -H 'Content-Type: application/json' \\
  -d '{"method":"GET","url":"https://api.github.com/users/octocat"}' \\
  http://localhost:8080/api/v1/requests`,

  sdk: `// Egress Request Forwarding using Go SDK
package main

import (
\t"context"
\t"fmt"
\t"github.com/beremaran/straw/v2/sdk"
)

func main() {
\tclient := sdk.NewClient("http://localhost:8080", "sk_example_requester_secret")
\t
\tresp, err := client.Do(context.Background(), sdk.Request{
\t\tMethod: "GET",
\t\tURL:    "https://api.github.com/users/octocat",
\t})
\tif err != nil {
\t\tpanic(err)
\t}
\t
\tfmt.Printf("Status: %d, Response Envelope: %+v\\n", resp.Status, resp.Body)
}`,

  stream: `// Streaming Request Forwarding using Go SDK
package main

import (
\t"context"
\t"fmt"
\t"io"
\t"github.com/beremaran/straw/v2/sdk"
)

func main() {
\tclient := sdk.NewClient("http://localhost:8080", "sk_example_requester_secret")
\t
\tstream, _ := client.DoStream(context.Background(), sdk.Request{
\t\tMethod: "GET",
\t\tURL:    "https://speed.cloudflare.com/__down?bytes=10000",
\t})
\tdefer stream.Close()

\tfor {
\t\tframe, err := stream.Next()
\t\tif err == io.EOF {
\t\t\tbreak
\t\t}
\t\t
\t\tif frame.Type == sdk.StreamFrameBody {
\t\t\tfmt.Print(string(frame.Body))
\t\t}
\t}
}`
};

export default function Home() {
  const { siteConfig } = useDocusaurusContext();
  const [activeTab, setActiveTab] = useState('curl');

  return (
    <Layout
      title={siteConfig.title}
      description="Straw: A high-performance distributed HTTP/HTTPS egress proxy control plane and worker system.">
      <HomepageHeader />
      
      <main>
        {/* Features Section */}
        <section className={styles.section}>
          <div className="container">
            <h2 className={styles.sectionTitle}>Built for Scale, Security, and Governance</h2>
            <div className={styles.featuresGrid}>
              
              <div className={styles.featureCard}>
                <div className={styles.featureIcon}>🛡️</div>
                <h3 className={styles.featureCardTitle}>Cryptographic Isolation</h3>
                <p className={styles.featureCardDesc}>
                  Workers authenticate via Ed25519-signed registration handshakes. Replay protection prevents rogue node hijacking.
                </p>
              </div>

              <div className={styles.featureCard}>
                <div className={styles.featureIcon}>⚡</div>
                <h3 className={styles.featureCardTitle}>TLS Fingerprinting</h3>
                <p className={styles.featureCardDesc}>
                  Apply browser fingerprint profiles (e.g. Chrome 120) to bypass target server protection and prevent proxy detection.
                </p>
              </div>

              <div className={styles.featureCard}>
                <div className={styles.featureIcon}>🔗</div>
                <h3 className={styles.featureCardTitle}>Session Stickiness</h3>
                <p className={styles.featureCardDesc}>
                  Enforce request affinity to route sequential requests to the same worker, optimizing cache and persistence contexts.
                </p>
              </div>

              <div className={styles.featureCard}>
                <div className={styles.featureIcon}>📊</div>
                <h3 className={styles.featureCardTitle}>Admission Guardrails</h3>
                <p className={styles.featureCardDesc}>
                  Enforce destination policy blocklists (CIDR/Host), request size limits, and monthly bandwidth allocations.
                </p>
              </div>

              <div className={styles.featureCard}>
                <div className={styles.featureIcon}>🌐</div>
                <h3 className={styles.featureCardTitle}>Rich API Telemetry</h3>
                <p className={styles.featureCardDesc}>
                  Asynchronous ClickHouse event persistence for historical analytics, config audit trail logging, and debugging.
                </p>
              </div>

              <div className={styles.featureCard}>
                <div className={styles.featureIcon}>🔄</div>
                <h3 className={styles.featureCardTitle}>Rollback & Snapshots</h3>
                <p className={styles.featureCardDesc}>
                  Durable Postgres configuration storage tracks modification history, enabling instant tenant policy rollbacks.
                </p>
              </div>

            </div>
          </div>
        </section>

        {/* Code Preview Section */}
        <section className={styles.sectionAlt}>
          <div className="container">
            <h2 className={styles.sectionTitle}>Simple to Integrate</h2>
            <div className={styles.codePreviewContainer}>
              <div className={styles.tabHeader}>
                <button 
                  className={`${styles.tabButton} ${activeTab === 'curl' ? styles.tabButtonActive : ''}`}
                  onClick={() => setActiveTab('curl')}>
                  cURL REST API
                </button>
                <button 
                  className={`${styles.tabButton} ${activeTab === 'sdk' ? styles.tabButtonActive : ''}`}
                  onClick={() => setActiveTab('sdk')}>
                  Go Client SDK
                </button>
                <button 
                  className={`${styles.tabButton} ${activeTab === 'stream' ? styles.tabButtonActive : ''}`}
                  onClick={() => setActiveTab('stream')}>
                  Binary Streaming
                </button>
              </div>
              <div className={styles.codeBox}>
                <pre className={styles.codeBoxPre}>
                  <code>{CODE_TEMPLATES[activeTab]}</code>
                </pre>
              </div>
            </div>
          </div>
        </section>

        {/* Quick Architecture Section */}
        <section className={styles.section}>
          <div className="container">
            <h2 className={styles.sectionTitle}>Request Flow Architecture</h2>
            <div className={styles.diagramBox}>
              <div className={styles.diagramFlow}>
                <div className={styles.diagramNode}>
                  <div className={styles.diagramNodeTitle}>Client Workloads</div>
                  <small>Submits HTTP/HTTPS egress targets to forwarding API</small>
                </div>
                <div className={styles.diagramArrow}>&rarr;</div>
                <div className={styles.diagramNode}>
                  <div className={styles.diagramNodeTitle}>Control Plane</div>
                  <small>Validates policies, auth, rate-limits, and routing</small>
                </div>
                <div className={styles.diagramArrow}>&rarr;</div>
                <div className={styles.diagramNode}>
                  <div className={styles.diagramNodeTitle}>NATS Scheduler</div>
                  <small>Schedules assignments to workers based on pool/tags</small>
                </div>
                <div className={styles.diagramArrow}>&rarr;</div>
                <div className={styles.diagramNode}>
                  <div className={styles.diagramNodeTitle}>Egress Workers</div>
                  <small>Applies TLS fingerprints and executes upstream calls</small>
                </div>
              </div>
            </div>
          </div>
        </section>

      </main>
    </Layout>
  );
}
