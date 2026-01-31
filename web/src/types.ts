export interface Application {
  id: string;
  name: string;
  description: string;
  url: string;
  icon: string;
  category: string;
}

export interface AppConfig {
  applications: Application[];
}
