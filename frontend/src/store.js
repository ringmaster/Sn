import { writable } from 'svelte/store';

// Create a writable store
export const dataStore = writable({loggedIn: false, username: null, title: 'Sn'});

// Function to fetch data from the JSON endpoint
export async function fetchData() {
  try {
    const response = await fetch('./api');
    if (!response.ok) {
        dataStore.set({loggedIn: false, username: null, title: 'Sn'});
        throw new Error('Failed to fetch data');
    }
    const data = await response.json();
    dataStore.set(data);
  } catch (error) {
    dataStore.set({loggedIn: false, username: null, title: 'Sn'});
    console.error('Error fetching data:', error);
  }
}