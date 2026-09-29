const puppeteer = require('puppeteer');
const path = require('path');
const fs = require('fs');

async function generateAll() {
    const browser = await puppeteer.launch({
        headless: 'new',
        args: ['--no-sandbox', '--disable-setuid-sandbox']
    });

    const files = [
        { html: 'app-icon.html', png: 'app-icon.png', width: 512, height: 512 },
        { html: 'feature-graphic.html', png: 'feature-graphic.png', width: 1024, height: 500 },
        { html: 'screenshot-dashboard.html', png: 'screenshot-1-dashboard.png', width: 1080, height: 1920 },
        { html: 'screenshot-nodes.html', png: 'screenshot-2-nodes.png', width: 1080, height: 1920 },
        { html: 'screenshot-playbooks.html', png: 'screenshot-3-playbooks.png', width: 1080, height: 1920 },
        { html: 'screenshot-settings.html', png: 'screenshot-4-settings.png', width: 1080, height: 1920 },
        { html: 'screenshot-dashboard.html', png: 'tablet-7-1-dashboard.png', width: 1600, height: 2560 },
        { html: 'screenshot-nodes.html', png: 'tablet-7-2-nodes.png', width: 1600, height: 2560 },
        { html: 'screenshot-dashboard.html', png: 'tablet-10-1-dashboard.png', width: 1920, height: 3200 },
        { html: 'screenshot-nodes.html', png: 'tablet-10-2-nodes.png', width: 1920, height: 3200 },
    ];

    for (const file of files) {
        const page = await browser.newPage();

        // Read HTML content
        const htmlContent = fs.readFileSync(path.join(__dirname, file.html), 'utf8');

        // Set viewport with device scale factor
        await page.setViewport({
            width: file.width,
            height: file.height,
            deviceScaleFactor: 1
        });

        // Set content with base URL
        await page.setContent(htmlContent, {
            waitUntil: 'networkidle0'
        });

        // Wait for rendering
        await new Promise(resolve => setTimeout(resolve, 200));

        // Screenshot with exact clip region to avoid white borders
        await page.screenshot({
            path: file.png,
            type: 'png',
            clip: {
                x: 0,
                y: 0,
                width: file.width,
                height: file.height
            }
        });

        console.log(`✓ ${file.png} (${file.width}x${file.height})`);
        await page.close();
    }

    await browser.close();
    console.log('\nDone!');
}

generateAll().catch(console.error);