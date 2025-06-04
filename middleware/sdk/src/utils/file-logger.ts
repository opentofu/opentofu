import fs from "node:fs";

export class FileLogger {
  constructor(private logFile: string) {}

  log(message: string): void {
    const timestamp = new Date().toISOString();
    fs.appendFileSync(this.logFile, `[${timestamp}] ${message}\n`);
  }

  logJSON(data: any): void {
    this.log(JSON.stringify(data));
  }
}
