import {id as pluginId} from './manifest';
import Client from './client';

let activityFunc: () => void;
let lastActivityTime = 0;
const activityTimeout = 1 * 60 * 1000; // 1 min

export default class Plugin {
    // eslint-disable-next-line no-unused-vars
    public initialize(): void {
        // @see https://developers.mattermost.com/extend/plugins/webapp/reference/
        activityFunc = (): void => {
            const now = new Date().getTime();
            if (now - lastActivityTime > activityTimeout) {
                try {
                    const resp = (new Client()).getConnected();
                } catch (e) {}
                lastActivityTime = now;
            }
        };

        document.addEventListener('click', activityFunc);
    }

    public deinitialize(): void {
        document.removeEventListener('click', activityFunc);
    }
}

window.registerPlugin(pluginId, new Plugin());
