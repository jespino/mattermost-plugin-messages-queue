import {Client4} from 'mattermost-redux/client';
import {ClientError} from 'mattermost-redux/client/client4';

export default class Client {
      constructor() {
          this.url = '/plugins/com.github.jespino.messages-queue/';
      }
  
      getConnected = async () => {
          return this.doPost(this.url);
      }

      doPost = async (url, body, headers = {}) => {
          headers['X-Timezone-Offset'] = new Date().getTimezoneOffset();
  
          const options = {
              method: 'post',
              body: JSON.stringify(body),
              headers,
          };
  
          const response = await fetch(url, Client4.getOptions(options));
  
          if (response.ok) {
              return response.json();
          }
  
          const text = await response.text();
  
          throw new ClientError(Client4.url, {
              message: text || '',
              status_code: response.status,
              url,
          });
      }
}
