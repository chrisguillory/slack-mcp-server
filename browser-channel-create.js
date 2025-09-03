// Browser automation using Puppeteer
// Install: npm install puppeteer

const puppeteer = require('puppeteer');

async function createSlackChannel(channelName, isPrivate = false) {
  // Launch browser with existing user data to use your logged-in session
  const browser = await puppeteer.launch({
    headless: false, // Set to true once it works
    userDataDir: '/path/to/your/chrome/profile', // Use your actual Chrome profile
    // On Mac: ~/Library/Application Support/Google/Chrome/Default
  });

  const page = await browser.newPage();
  
  // Navigate to Slack
  await page.goto('https://app.slack.com/client/E08K7E7N092/'); // Your Enterprise ID
  
  // Wait for Slack to load
  await page.waitForSelector('[data-qa="channel_sidebar"]', { timeout: 10000 });
  
  // Click "Add channels" button
  await page.click('[data-qa="add-channels-button"]');
  
  // Click "Create a channel"
  await page.click('[data-qa="menu_item_button"][data-name="menu_create_channel"]');
  
  // Wait for modal
  await page.waitForSelector('[data-qa="channel_create_modal"]');
  
  // Enter channel name
  await page.type('[data-qa="channel_create_modal_input"]', channelName);
  
  // Toggle private if needed
  if (isPrivate) {
    await page.click('[data-qa="channel_create_modal_private_toggle"]');
  }
  
  // Click Create button
  await page.click('[data-qa="channel_create_modal_submit"]');
  
  // Wait for navigation to new channel
  await page.waitForNavigation();
  
  console.log(`Created channel: ${channelName}`);
  
  await browser.close();
}

// Usage
createSlackChannel('test-browser-automation', true)
  .catch(console.error);