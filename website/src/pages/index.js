import React from 'react';
import Link from '@docusaurus/Link';
import useDocusaurusContext from '@docusaurus/useDocusaurusContext';
import Layout from '@theme/Layout';
import Heading from '@theme/Heading';
import styles from './index.module.css';

const REQUEST = `curl -sS \\
  -H 'Content-Type: application/json' \\
  -d '{"method":"GET","url":"https://example.com"}' \\
  http://localhost:8080/api/v1/requests`;

function HomepageHeader() {
  const {siteConfig} = useDocusaurusContext();
  return (
    <header className={styles.heroSection}>
      <div className={styles.heroGridBg} />
      <div className={styles.glowOrb} />
      <div className={styles.heroContent}>
        <Heading as="h1" className={styles.heroTitle}>{siteConfig.title}</Heading>
        <p className={styles.heroTagline}>{siteConfig.tagline}</p>
        <div className={styles.heroButtons}>
          <Link className={styles.primaryButton} to="/docs/quickstart">Run it locally &rarr;</Link>
          <Link className={styles.secondaryButton} to="/docs/architecture">See how it works</Link>
        </div>
      </div>
    </header>
  );
}

export default function Home() {
  const {siteConfig} = useDocusaurusContext();
  return (
    <Layout title={siteConfig.title} description="A small, self-hosted HTTP/HTTPS egress proxy.">
      <HomepageHeader />
      <main>
        <section className={styles.section}>
          <div className="container">
            <h2 className={styles.sectionTitle}>Three services, one clear job</h2>
            <div className={styles.featuresGrid}>
              <article className={styles.featureCard}>
                <h3 className={styles.featureCardTitle}>Control</h3>
                <p className={styles.featureCardDesc}>A small request API that validates input and assigns healthy workers.</p>
              </article>
              <article className={styles.featureCard}>
                <h3 className={styles.featureCardTitle}>NATS</h3>
                <p className={styles.featureCardDesc}>The only required backing service, used for discovery and request transport.</p>
              </article>
              <article className={styles.featureCard}>
                <h3 className={styles.featureCardTitle}>Egress</h3>
                <p className={styles.featureCardDesc}>Independently scalable workers that make outbound HTTP and HTTPS requests.</p>
              </article>
            </div>
          </div>
        </section>
        <section className={styles.sectionAlt}>
          <div className="container">
            <h2 className={styles.sectionTitle}>From clone to request</h2>
            <div className={styles.codePreviewContainer}>
              <div className={styles.codeBox}>
                <pre className={styles.codeBoxPre}><code>{`make dev\n\n${REQUEST}`}</code></pre>
              </div>
            </div>
          </div>
        </section>
        <section className={styles.section}>
          <div className="container">
            <h2 className={styles.sectionTitle}>Open-source boundaries</h2>
            <div className={styles.diagramBox}>
              <p className={styles.featureCardDesc}>
                One deployment is one trust boundary. No tenants, RBAC, billing, quotas, configuration database,
                or analytics database. Local development is the primary path; production files are examples you adapt.
              </p>
            </div>
          </div>
        </section>
      </main>
    </Layout>
  );
}
