import { fetchData, dataStore } from '../store';

export const prerender = true;
export const ssr = false;

export async function load({ fetch }) {
  const response = await fetch('./api');
  const data = await response.json();

  // Prime the store with the fetched data
  dataStore.set(data);

  return {
    props: {
      data
    }
  };
}

