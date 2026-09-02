import {TrueNASService} from "../bindings/github.com/ironpark/toolz/desktop/charmtrue";

const message = document.getElementById("message")!;

document.getElementById("connect")!.addEventListener("click", async () => {
    const info = await TrueNASService.AppInfo();
    message.textContent = `${info.name} ${info.version}: ${info.status}`;
});
